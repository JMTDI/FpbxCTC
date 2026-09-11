package main

// Version is the application version. It is overridden at build time via
// `-ldflags "-X main.Version=x.y.z"` by the release workflow
// (.github/workflows/build.yml). Defaults to "dev" for local builds.
var Version = "3.3.1"
