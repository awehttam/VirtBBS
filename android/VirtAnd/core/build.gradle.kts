// VirtAnd — core/build.gradle.kts
// Pure Kotlin/JVM module: no Android dependency. Build/test without the Android SDK.
plugins {
    `java-library`
    alias(libs.plugins.kotlin.jvm)
    alias(libs.plugins.kotlin.serialization)
}

kotlin {
    jvmToolchain(17)
}

dependencies {
    implementation(kotlin("stdlib"))
    // api: :app builds JsonObject params for UserApiClient via types from :core
    api(libs.kotlinx.serialization.json)
    testImplementation(kotlin("test"))
    testRuntimeOnly("org.junit.platform:junit-platform-launcher")
}

tasks.test {
    useJUnitPlatform()
}