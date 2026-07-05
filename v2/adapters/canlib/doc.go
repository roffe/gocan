// Package canlib registers Kvaser CAN channels ("CANlib #0 ...", one per
// attached channel, virtual channels skipped) via Kvaser's CANlib SDK.
// The implementation is opt-in with the "canlib" build tag since it needs
// the Kvaser driver/SDK installed; without the tag the package is empty.
package canlib
