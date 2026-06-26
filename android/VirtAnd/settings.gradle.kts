// VirtAnd — settings.gradle.kts
//
// Two modules:
//   core — pure Kotlin/JVM business logic (API client, QWK packet parsing).
//   app  — Android application (Room, WorkManager, Compose UI).
//
// Build tooling matches ClonesApp (see ../CLAUDE.md and ../../CLAUDE.md).
enableFeaturePreview("STABLE_CONFIGURATION_CACHE")

pluginManagement {
    repositories {
        google()
        mavenCentral()
        gradlePluginPortal()
    }
}
plugins {
    id("org.gradle.toolchains.foojay-resolver-convention") version "1.0.0"
}
dependencyResolutionManagement {
    repositoriesMode.set(RepositoriesMode.FAIL_ON_PROJECT_REPOS)
    repositories {
        google()
        mavenCentral()
    }
}

rootProject.name = "VirtAnd"
include(":core")
include(":app")