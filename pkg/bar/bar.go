package bar

import (
	"github.com/k0kubun/go-ansi"
	"github.com/schollz/progressbar/v3"
)

func New(length int, text string) *progressbar.ProgressBar {
	return progressbar.NewOptions(
		length,
		progressbar.OptionSetWriter(ansi.NewAnsiStdout()),
		progressbar.OptionEnableColorCodes(true),
		progressbar.OptionShowBytes(true),
		progressbar.OptionSetWidth(20),
		progressbar.OptionSetDescription(text),
		// progressbar.OptionSetDescription("[cyan][1/1][reset] "+text),
		progressbar.OptionSetTheme(progressbar.Theme{
			Saucer:        "[green]=[reset]",
			SaucerHead:    "[green]>[reset]",
			SaucerPadding: " ",
			BarStart:      "[",
			BarEnd:        "]",
		}),
	)
}
