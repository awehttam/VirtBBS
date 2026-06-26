// VirtAnd — SyncEngine.kt
//
// The single "synchronize" entry point, run either by the user tapping
// Sync or by SyncWorker in the background. Per the plan: VirtAnd runs
// mostly offline, and nothing leaves or enters the device except during
// this explicit step.
//
// Order matters here and is deliberate:
//   1. QWK download + import new messages into the local cache.
//   2. File catalog refresh (dirs + per-dir listings).
//   3. Execute queued downloads (now that the catalog is fresh).
//   4. Execute queued uploads.
//   5. Nodelist version check per subscribed network.
//   6. Build + upload a REP packet for queued replies, then clear the
//      queue only on confirmed success — never drop queued replies on a
//      network failure partway through.
package io.virtbbs.virtand.sync

import android.content.Context
import io.virtbbs.virtand.core.QwkReply
import io.virtbbs.virtand.core.UserApiClient
import io.virtbbs.virtand.core.buildRepPacket
import io.virtbbs.virtand.core.parseQwkPacket
import io.virtbbs.virtand.data.AppDatabase
import io.virtbbs.virtand.data.entities.CachedMessageEntity
import io.virtbbs.virtand.data.entities.ConferenceEntity
import io.virtbbs.virtand.data.entities.FileDirEntity
import io.virtbbs.virtand.data.entities.FileEntryEntity
import io.virtbbs.virtand.data.entities.NodelistVersionEntity
import io.virtbbs.virtand.settings.SettingsStore
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.int
import kotlinx.serialization.json.jsonArray
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import kotlinx.serialization.json.long
import java.util.Base64

/** Outcome of one synchronize pass, for display in the UI / WorkManager logs. */
data class SyncResult(
    val newMessages: Int,
    val repliesUploaded: Int,
    val filesUploaded: Int,
    val filesDownloaded: Int,
    val nodelistsChanged: List<String>,
    val error: String? = null,
)

class SyncEngine(private val context: Context) {
    private val db = AppDatabase.get(context)
    private val settings = SettingsStore(context)

    suspend fun synchronize(): SyncResult = withContext(Dispatchers.IO) {
        val cfg = settings.snapshot()
        if (cfg.host.isBlank() || cfg.token.isBlank()) {
            return@withContext SyncResult(0, 0, 0, 0, emptyList(), error = "Not configured — set host/token first.")
        }
        val api = UserApiClient(host = cfg.host, port = cfg.userApiPort, token = cfg.token)

        try {
            refreshConferences(api)
            val newMessages = downloadAndImportQwk(api)
            refreshFileCatalog(api)
            val filesDownloaded = executeQueuedDownloads(api)
            val filesUploaded = executeQueuedUploads(api)
            val nodelistsChanged = checkNodelists(api, cfg.subscribedNetworks)
            val repliesUploaded = uploadQueuedReplies(api)

            SyncResult(newMessages, repliesUploaded, filesUploaded, filesDownloaded, nodelistsChanged)
        } catch (e: Exception) {
            SyncResult(0, 0, 0, 0, emptyList(), error = e.message ?: e.toString())
        }
    }

    private fun refreshConferences(api: UserApiClient) {
        val result = api.call("conferences.list") ?: return
        val entities = result.jsonArray.map { c ->
            val o = c.jsonObject
            ConferenceEntity(
                id = o["ID"]!!.jsonPrimitive.int,
                name = o["Name"]!!.jsonPrimitive.content,
                description = o["Description"]?.jsonPrimitive?.content ?: "",
                readSec = o["ReadSec"]?.jsonPrimitive?.int ?: 0,
                writeSec = o["WriteSec"]?.jsonPrimitive?.int ?: 0,
            )
        }
        runBlockingDb { db.conferenceDao().upsertAll(entities) }
    }

    /** Downloads a QWK packet (new-since-last-sync messages, all conferences) and caches it locally. */
    private fun downloadAndImportQwk(api: UserApiClient): Int {
        val result = api.call("qwk.download") ?: return 0
        val b64 = result.jsonObject["data"]?.jsonPrimitive?.content ?: return 0
        val zipBytes = Base64.getDecoder().decode(b64)
        val messages = parseQwkPacket(zipBytes)

        val entities = messages.map {
            CachedMessageEntity(
                conferenceId = it.conferenceId,
                msgNumber = it.msgNumber,
                date = it.date,
                time = it.time,
                toName = it.toName,
                fromName = it.fromName,
                subject = it.subject,
                body = it.body,
            )
        }
        runBlockingDb { db.messageDao().insertAll(entities) }
        return entities.size
    }

    private fun refreshFileCatalog(api: UserApiClient) {
        val dirsResult = api.call("files.dirs.list") ?: return
        val dirs = dirsResult.jsonArray.map { d ->
            val o = d.jsonObject
            FileDirEntity(
                id = o["ID"]!!.jsonPrimitive.long,
                name = o["Name"]!!.jsonPrimitive.content,
                description = o["Description"]?.jsonPrimitive?.content ?: "",
                readSec = o["ReadSec"]?.jsonPrimitive?.int ?: 0,
                uploadSec = o["UploadSec"]?.jsonPrimitive?.int ?: 0,
            )
        }
        runBlockingDb { db.fileDao().upsertDirs(dirs) }

        for (dir in dirs) {
            val filesResult = api.call("files.list", JsonObject(mapOf("DirID" to JsonPrimitive(dir.id)))) ?: continue
            val files = filesResult.jsonArray.map { f ->
                val o = f.jsonObject
                FileEntryEntity(
                    id = o["ID"]!!.jsonPrimitive.long,
                    dirId = dir.id,
                    filename = o["Filename"]!!.jsonPrimitive.content,
                    size = o["Size"]?.jsonPrimitive?.long ?: 0,
                    description = o["Description"]?.jsonPrimitive?.content ?: "",
                    uploader = o["Uploader"]?.jsonPrimitive?.content ?: "",
                    uploadDate = o["UploadDate"]?.jsonPrimitive?.content ?: "",
                )
            }
            runBlockingDb {
                db.fileDao().clearFiles(dir.id)
                db.fileDao().upsertFiles(files)
            }
        }
    }

    private fun executeQueuedDownloads(api: UserApiClient): Int {
        val queued = runBlockingDb { db.fileDao().allQueuedDownloads() }
        var count = 0
        for (q in queued) {
            try {
                val result = api.call(
                    "files.download",
                    JsonObject(mapOf("DirID" to JsonPrimitive(q.dirId), "Filename" to JsonPrimitive(q.filename))),
                )
                val b64 = result?.jsonObject?.get("data")?.jsonPrimitive?.content
                    ?: throw IllegalStateException("empty files.download response")
                val bytes = Base64.getDecoder().decode(b64)

                // App-specific external storage needs no runtime permission
                // on API 21+ and is visible to the user via a file manager.
                val dir = java.io.File(context.getExternalFilesDir(null), "downloads").apply { mkdirs() }
                java.io.File(dir, q.filename).writeBytes(bytes)

                runBlockingDb { db.fileDao().removeQueuedDownload(q) }
                count++
            } catch (_: Exception) {
                // Leave it queued — retry on next synchronize.
            }
        }
        return count
    }

    private fun executeQueuedUploads(api: UserApiClient): Int {
        val queued = runBlockingDb { db.fileDao().allQueuedUploads() }
        var count = 0
        for (q in queued) {
            try {
                val bytes = context.contentResolver.openInputStream(android.net.Uri.parse(q.localFileUri))
                    ?.use { it.readBytes() } ?: continue
                val b64 = Base64.getEncoder().encodeToString(bytes)
                api.call(
                    "files.upload",
                    JsonObject(
                        mapOf(
                            "DirID" to JsonPrimitive(q.dirId),
                            "Filename" to JsonPrimitive(q.filename),
                            "Description" to JsonPrimitive(q.description),
                            "Data" to JsonPrimitive(b64),
                        )
                    ),
                )
                runBlockingDb { db.fileDao().removeQueuedUpload(q) }
                count++
            } catch (_: Exception) {
                // Leave it queued — retry on next synchronize.
            }
        }
        return count
    }

    private fun checkNodelists(api: UserApiClient, networks: List<String>): List<String> {
        val changed = mutableListOf<String>()
        for (network in networks) {
            try {
                val result = api.call("fido.nodelist.version", JsonObject(mapOf("network" to JsonPrimitive(network))))
                    ?: continue
                val o = result.jsonObject
                val importedAt = o["imported_at"]?.jsonPrimitive?.content ?: continue
                val nodeCount = o["node_count"]?.jsonPrimitive?.int ?: 0

                val cached = runBlockingDb { db.nodelistDao().get(network) }
                if (cached == null || cached.importedAt != importedAt || cached.nodeCount != nodeCount) {
                    runBlockingDb { db.nodelistDao().upsert(NodelistVersionEntity(network, importedAt, nodeCount)) }
                    changed.add(network)
                }
            } catch (_: Exception) {
                // Skip this network this round, try again next sync.
            }
        }
        return changed
    }

    /** Builds and uploads one REP packet from every queued reply, clearing the queue only on confirmed success. */
    private fun uploadQueuedReplies(api: UserApiClient): Int {
        val queued = runBlockingDb { db.messageDao().allQueuedReplies() }
        if (queued.isEmpty()) return 0

        val replies = queued.map {
            QwkReply(
                conferenceId = it.conferenceId,
                refNum = it.refNum,
                toName = it.toName,
                fromName = "", // server attributes authorship from the authenticated token, not this field
                subject = it.subject,
                body = it.body,
            )
        }
        val repBytes = buildRepPacket(replies)
        val b64 = Base64.getEncoder().encodeToString(repBytes)
        api.call("qwk.upload", JsonObject(mapOf("Data" to JsonPrimitive(b64))))

        runBlockingDb { db.messageDao().removeQueuedReplies(queued.map { it.id }) }
        return queued.size
    }

    // Room's suspend DAO methods need a coroutine scope; since this whole
    // engine already runs on Dispatchers.IO via the outer withContext, a
    // simple runBlocking-style bridge here is fine (no UI thread involved).
    private fun <T> runBlockingDb(block: suspend () -> T): T =
        kotlinx.coroutines.runBlocking { block() }
}
