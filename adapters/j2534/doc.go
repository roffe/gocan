// Package j2534 registers J2534 PassThru DLL interfaces found in the Windows
// registry ("J2534 #0 <name>", one per installed DLL). Opt-in with the
// "j2534" build tag since it drives vendor DLLs; without the tag (or off
// Windows) the package is empty. For J2534 devices served over the network,
// see adapters/gateway instead.
package j2534
