// VirtAnd — MainActivity.kt
//
// Minimal UI per the plan: message reader/composer against the local
// cache, file browser + upload/download queue, settings, and a manual
// "Synchronize" action. Everything here reads/writes Room directly via
// MainViewModel — no network call happens outside SyncEngine.synchronize().
package io.virtbbs.virtand.ui

import android.os.Bundle
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.activity.viewModels
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Refresh
import androidx.compose.material3.Button
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.ListItem
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.NavigationBar
import androidx.compose.material3.NavigationBarItem
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TextField
import androidx.compose.material3.TopAppBar
import androidx.compose.runtime.Composable
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.dp
import androidx.navigation.NavHostController
import androidx.navigation.compose.NavHost
import androidx.navigation.compose.composable
import androidx.navigation.compose.currentBackStackEntryAsState
import androidx.navigation.compose.rememberNavController

class MainActivity : ComponentActivity() {
    private val viewModel: MainViewModel by viewModels()

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        setContent {
            MaterialTheme {
                Surface(modifier = Modifier.fillMaxSize()) {
                    VirtAndApp(viewModel)
                }
            }
        }
    }
}

private sealed class Tab(val route: String, val label: String) {
    data object Messages : Tab("messages", "Messages")
    data object Files : Tab("files", "Files")
    data object Settings : Tab("settings", "Settings")
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun VirtAndApp(viewModel: MainViewModel) {
    val nav: NavHostController = rememberNavController()
    val syncing by viewModel.syncing.collectAsState()
    val syncStatus by viewModel.syncStatus.collectAsState()

    Scaffold(
        topBar = {
            TopAppBar(
                title = { Text("VirtAnd") },
                actions = {
                    IconButton(onClick = { viewModel.synchronize() }, enabled = !syncing) {
                        Icon(Icons.Filled.Refresh, contentDescription = "Synchronize")
                    }
                },
            )
        },
        bottomBar = {
            val backStackEntry by nav.currentBackStackEntryAsState()
            val current = backStackEntry?.destination?.route
            NavigationBar {
                listOf(Tab.Messages, Tab.Files, Tab.Settings).forEach { tab ->
                    NavigationBarItem(
                        selected = current == tab.route,
                        onClick = { nav.navigate(tab.route) },
                        icon = {},
                        label = { Text(tab.label) },
                    )
                }
            }
        },
    ) { scaffoldPadding ->
        Column(modifier = Modifier.padding(scaffoldPadding)) {
            Text(syncStatus, modifier = Modifier.padding(8.dp))
            NavHost(navController = nav, startDestination = Tab.Messages.route) {
                composable(Tab.Messages.route) { MessagesScreen(viewModel) }
                composable(Tab.Files.route) { FilesScreen(viewModel) }
                composable(Tab.Settings.route) { SettingsScreen(viewModel) }
            }
        }
    }
}

@Composable
private fun MessagesScreen(viewModel: MainViewModel) {
    val conferences by viewModel.conferences.collectAsState()
    var selectedConference by remember { mutableStateOf<Int?>(null) }

    if (selectedConference == null) {
        LazyColumn {
            items(conferences) { conf ->
                ListItem(
                    headlineContent = { Text(conf.name) },
                    supportingContent = { Text(conf.description) },
                    modifier = Modifier
                        .padding(4.dp)
                        .clickable { selectedConference = conf.id },
                )
            }
        }
    } else {
        val conferenceId = selectedConference!!
        val messages by viewModel.messagesFor(conferenceId).collectAsState(initial = emptyList())
        var replyTo by remember { mutableStateOf<Int?>(null) }
        var replyBody by remember { mutableStateOf("") }

        Button(onClick = { selectedConference = null }) { Text("< Back to Conferences") }
        LazyColumn {
            items(messages) { msg ->
                ListItem(
                    headlineContent = { Text("${msg.fromName} -> ${msg.toName}: ${msg.subject}") },
                    supportingContent = { Text(msg.body.take(200)) },
                    modifier = Modifier.padding(4.dp),
                )
                if (replyTo == msg.msgNumber) {
                    TextField(
                        value = replyBody,
                        onValueChange = { replyBody = it },
                        modifier = Modifier.padding(8.dp),
                        label = { Text("Reply") },
                    )
                    Button(onClick = {
                        viewModel.queueReply(conferenceId, msg.msgNumber, msg.fromName, "Re: ${msg.subject}", replyBody)
                        replyTo = null
                        replyBody = ""
                    }) { Text("Queue Reply (sent on next Synchronize)") }
                } else {
                    Button(onClick = { replyTo = msg.msgNumber }) { Text("Reply") }
                }
            }
        }
    }
}

@Composable
private fun FilesScreen(viewModel: MainViewModel) {
    val dirs by viewModel.fileDirs.collectAsState()
    var selectedDir by remember { mutableStateOf<Long?>(null) }

    if (selectedDir == null) {
        LazyColumn {
            items(dirs) { dir ->
                ListItem(
                    headlineContent = { Text(dir.name) },
                    supportingContent = { Text(dir.description) },
                    modifier = Modifier
                        .padding(4.dp)
                        .clickable { selectedDir = dir.id },
                )
            }
        }
    } else {
        val dirId = selectedDir!!
        val files by viewModel.filesFor(dirId).collectAsState(initial = emptyList())
        val context = androidx.compose.ui.platform.LocalContext.current
        val pickFile = androidx.activity.compose.rememberLauncherForActivityResult(
            androidx.activity.result.contract.ActivityResultContracts.OpenDocument()
        ) { uri ->
            if (uri != null) {
                // The picker's URI grant is temporary unless persisted — uploads
                // are only executed on the next "synchronize", which may happen
                // well after this activity is gone (even via WorkManager after a
                // restart), so the read permission must survive that long.
                context.contentResolver.takePersistableUriPermission(
                    uri, android.content.Intent.FLAG_GRANT_READ_URI_PERMISSION
                )
                val filename = uri.lastPathSegment?.substringAfterLast('/') ?: "upload.bin"
                viewModel.queueUpload(dirId, uri.toString(), filename, "")
            }
        }

        Button(onClick = { selectedDir = null }) { Text("< Back to File Areas") }
        Button(onClick = { pickFile.launch(arrayOf("*/*")) }) { Text("Pick File to Upload") }
        LazyColumn {
            items(files) { f ->
                ListItem(
                    headlineContent = { Text(f.filename) },
                    supportingContent = { Text("${f.size} bytes — ${f.description}") },
                    trailingContent = {
                        Button(onClick = { viewModel.queueDownload(dirId, f.filename) }) {
                            Text("Queue DL")
                        }
                    },
                    modifier = Modifier.padding(4.dp),
                )
            }
        }
    }
}

@Composable
private fun SettingsScreen(viewModel: MainViewModel) {
    val host by viewModel.settings.host.collectAsState(initial = "")
    val token by viewModel.settings.token.collectAsState(initial = "")
    val port by viewModel.settings.userApiPort.collectAsState(initial = 9998)
    val networks by viewModel.settings.subscribedNetworks.collectAsState(initial = listOf("FidoNet"))

    var hostField by remember(host) { mutableStateOf(host) }
    var portField by remember(port) { mutableStateOf(port.toString()) }
    var tokenField by remember(token) { mutableStateOf(token) }
    var networksField by remember(networks) { mutableStateOf(networks.joinToString(",")) }

    Column(modifier = Modifier.padding(16.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
        Text("Generate a token on the BBS: Profile menu -> [T]okens -> [G]enerate.")
        TextField(value = hostField, onValueChange = { hostField = it }, label = { Text("Host") })
        TextField(value = portField, onValueChange = { portField = it }, label = { Text("User API Port") })
        TextField(value = tokenField, onValueChange = { tokenField = it }, label = { Text("API Token") })
        TextField(
            value = networksField,
            onValueChange = { networksField = it },
            label = { Text("Subscribed FidoNet networks (comma-separated)") },
        )
        Button(onClick = {
            viewModel.saveSettings(
                hostField,
                portField.toIntOrNull() ?: 9998,
                tokenField,
                networksField.split(",").map { it.trim() }.filter { it.isNotEmpty() },
            )
        }) { Text("Save") }
    }
}
