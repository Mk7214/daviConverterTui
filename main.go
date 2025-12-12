// main.go
package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
)

func main() {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		fmt.Println("ffmpeg not found in path, Intall it and try again")
		os.Exit(1)
	}
	// flag.Parse()
	final, err := RunTUI()
	if err != nil {
		fmt.Println("TUI error:", err)
		os.Exit(1)
	}
	if final.canceled {
		fmt.Println("Canceled by user; exiting.")
		os.Exit(0)
	}
	// format, err := formatValidator(formatFlag)
	// if err != nil {
	// 	os.Exit(1)
	// }
	format := final.format
	inputPath := final.value

	// inPathFlag := flag.String("in", "", "input video path")
	outPathFlag := flag.String("out", "", "output file path (optional)")
	// formatFlag := flag.String("format", "h264", "output format: h264|prores|dnxhd|wav")

	if err := isInputEmpty(final.value); err != nil {
		os.Exit(1)
	}

	// inputPath := *inPathFlag
	var outputPath string
	if *outPathFlag == "" {
		outputPath = defaultOutputPath(final.value, format)
	} else {
		outputPath = *outPathFlag
	}

	if err := Convert(inputPath, outputPath, format); err != nil {
		fmt.Println("ffmpeg failed: ", err)
		os.Exit(1)
	}
	fmt.Println("conversion finished successfully ")
}

// func main() {
// 	final, err := RunTUI()
// 	if err != nil {
// 		fmt.Println("TUI error:", err)
// 		os.Exit(1)
// 	}
// 	if final.canceled {
// 		fmt.Println("Canceled by user; exiting.")
// 		os.Exit(0)
// 	}
//
// 	fmt.Println("Input path:", final.value)
// 	fmt.Println("Selected format:", final.format)
// }
