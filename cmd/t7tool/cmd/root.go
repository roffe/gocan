package cmd

import (
	"context"
	"log"
	"os"
	"os/signal"
	"time"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "t7",
	Short: "T7 swish army tool",
	Long:  `Lorem ipsum and all the colors`,
	// Uncomment the following line if your bare application
	// has an action associated with it:
	// Run: func(cmd *cobra.Command, args []string) { },
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Setup interupt handler for ctrl-c
	quitChan := make(chan os.Signal, 1)
	signal.Notify(quitChan, os.Interrupt)
	// Create new canbus client

	go func() {
		s := <-quitChan
		log.Printf("got %v, exiting", s)
		cancel()
		// Failsafe if there is deadlocks
		<-time.After(45 * time.Second)
		log.Fatal("took to long to shutdown, forcefully exiting")
	}()

	err := rootCmd.ExecuteContext(ctx)
	if err != nil {
		os.Exit(1)
	}
}

var (
	comPort  string
	baudRate int
	debug    bool
)

func init() {
	log.SetFlags(0)
	rootCmd.PersistentFlags().StringVarP(&comPort, "port", "p", "*", "com-port, * = print available")
	rootCmd.PersistentFlags().IntVarP(&baudRate, "baudrate", "b", 115200, "baudrate")
	rootCmd.PersistentFlags().BoolVarP(&debug, "debug", "d", false, "debug mode")
	rootCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
