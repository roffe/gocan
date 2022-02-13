package cmd

import (
	"context"
	"log"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:          "t7",
	Short:        "T7 swish army tool",
	Long:         `Lorem ipsum and all the colors`,
	SilenceUsage: true,
	// Uncomment the following line if your bare application
	// has an action associated with it:
	// Run: func(cmd *cobra.Command, args []string) { },
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute(ctx context.Context) {
	//colorstring.Fprintln(ansi.NewAnsiStdout(), saab)
	rootCmd.ExecuteContext(ctx)
}

const (
	flagPort     = "port"
	flagBaudrate = "baudrate"
	flagDebug    = "debug"
	flagAdapter  = "adapter"
)

func init() {
	//	log.SetFlags(0)
	log.SetFlags(log.Lshortfile | log.LstdFlags)

	pf := rootCmd.PersistentFlags()
	pf.StringP(flagPort, "p", "*", "com-port, * = print available")
	pf.IntP(flagBaudrate, "b", 115200, "baudrate")
	pf.BoolP(flagDebug, "d", false, "debug mode")
	pf.StringP(flagAdapter, "a", "canusb", "what adapter to use")
	rootCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
