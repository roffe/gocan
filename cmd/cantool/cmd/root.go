package cmd

import (
	"context"
	"log"
	"os"

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
	switch os.Getenv("DEBUG") {
	case "true":
		log.SetFlags(log.Lshortfile | log.LstdFlags)
	default:
		log.SetFlags(0)
	}

	pf := rootCmd.PersistentFlags()
	pf.StringP(flagPort, "p", "*", "com-port, * = print available")
	pf.IntP(flagBaudrate, "b", 115200, "baudrate")
	pf.BoolP(flagDebug, "d", false, "debug mode")
	pf.StringP(flagAdapter, "a", "canusb", "what adapter to use")
	pf.StringP(flagCANRate, "c", "500", "CAN rate in kbit/s, shorts: pbus = 500, ibus = 47.619, t5 = 615.384")
	rootCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
