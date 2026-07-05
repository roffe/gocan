// Package drewtech drives the Drewtech Mongoose Pro GM II over its native
// serial protocol (no J2534 DLL needed) via gocan/v2/pkg/drewtech.
// Importing the package registers the "Drewtech Mongoose" adapter on Linux;
// on other platforms the package is empty.
package drewtech
