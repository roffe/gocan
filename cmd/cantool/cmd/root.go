package cmd

import (
	"context"
	"log"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "cantool",
	Short: "CANbus swish army tool",
	//Long:         `Lorem ipsum and all the colors`,
	SilenceUsage: true,
}

func Execute(ctx context.Context) {
	//colorstring.Fprintln(ansi.NewAnsiStdout(), saab)
	rootCmd.ExecuteContext(ctx)
}

const (
	flagPort     = "port"
	flagBaudrate = "baudrate"
	flagDebug    = "debug"
	flagAdapter  = "adapter"
	flagCANRate  = "canrate"
)

func init() {
	//	log.SetFlags(0)
	log.SetFlags(log.Lshortfile | log.LstdFlags | log.Lmicroseconds)

	pf := rootCmd.PersistentFlags()
	pf.StringP(flagPort, "p", "*", "com-port, * = print available")
	pf.IntP(flagBaudrate, "b", 115200, "baudrate")
	pf.BoolP(flagDebug, "d", false, "debug mode")
	pf.StringP(flagAdapter, "a", "canusb", "what adapter to use")
	pf.StringP(flagCANRate, "c", "500", "CAN rate in kbit/s, shorts: pbus = 500 (default), ibus = 47.619, t5 = 615.384")
	rootCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
