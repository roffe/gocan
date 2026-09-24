// Package j2534 registers J2534 PassThru interfaces ("J2534 #0 <name>", one
// per installed driver): on Windows the DLLs listed in the PassThruSupport
// registry key, on Linux the shared libraries described by ~/.passthru/*.json.
// Opt-in with the "j2534" build tag since it drives vendor libraries;
// without the tag, or on other platforms, the package is empty.
package j2534
