// VirtAnd — UserApiClient.kt
//
// JSON-over-TCP client for VirtBBS's internal/userapi — the same
// token-authenticated API used by VirtAnd. There's
// no HTTP server on the BBS side, just a raw newline-delimited-JSON socket
// protocol (see internal/userapi/server.go), so this opens a plain
// java.net.Socket per call rather than using an HTTP client library.
package io.virtbbs.virtand.core

import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonNull
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import java.io.BufferedReader
import java.io.InputStreamReader
import java.net.Socket
import java.nio.charset.StandardCharsets

class UserApiException(message: String) : Exception(message)

/**
 * One JSON/TCP call against internal/userapi. Opens a fresh connection,
 * sends one newline-delimited JSON request, reads one newline-delimited
 * JSON response line, and closes.
 */
class UserApiClient(
    var host: String = "127.0.0.1",
    var port: Int = 9998,
    var token: String = "",
) {
    private val json = Json { ignoreUnknownKeys = true }

    /** Sends [method] with [params] (any JSON-serializable value, or null) and returns the "result" field. */
    fun call(method: String, params: JsonElement? = null): JsonElement? {
        val req = buildJsonRequest(method, params, token)
        val reqLine = json.encodeToString(JsonObject.serializer(), req) + "\n"

        Socket(host, port).use { socket ->
            socket.soTimeout = 15_000
            val out = socket.getOutputStream()
            out.write(reqLine.toByteArray(StandardCharsets.UTF_8))
            out.flush()

            val reader = BufferedReader(InputStreamReader(socket.getInputStream(), StandardCharsets.UTF_8))
            val respLine = reader.readLine()
                ?: throw UserApiException("Empty response from server.")

            val resp = json.parseToJsonElement(respLine).jsonObject
            val error = resp["error"]
            if (error != null && error != JsonNull) {
                val msg = error.jsonPrimitive.content
                if (msg.isNotEmpty()) throw UserApiException(msg)
            }
            return resp["result"]
        }
    }

    /** Quick connectivity check — true if a token-authenticated call succeeds. */
    fun testConnection(): Boolean = try {
        call("conferences.list")
        true
    } catch (_: Exception) {
        false
    }
}

/**
 * Builds the {"method", "params", "auth": {"token"}} envelope as a
 * JsonObject, matching internal/userapi's Request{Method,Params,Auth} shape.
 */
fun buildJsonRequest(method: String, params: JsonElement?, token: String): JsonObject {
    val map = linkedMapOf<String, JsonElement>(
        "method" to JsonPrimitive(method),
        "auth" to JsonObject(mapOf("token" to JsonPrimitive(token))),
    )
    if (params != null) map["params"] = params
    return JsonObject(map)
}
