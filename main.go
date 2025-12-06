// main.go
package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

func main() {
	inPathFlag := flag.String("in", "", "input video path")
	outPathFlag := flag.String("out", "", "output file path (optional)")
	formatFlag := flag.String("format", "h264", "output format: h264|prores|dnxhd|wav")

	flag.Parse()

	// normalizing format to lowercase
	format := strings.ToLower(*formatFlag)
	// format validation
	switch format {
	case "h264", "prores", "dnxhd", "wav": // ok
	default:
		fmt.Println("invalid format:", format)
		fmt.Println("valid formats: h264, prores, dnxhd, wav")
		os.Exit(1)
	}

	if *inPathFlag == "" {
		fmt.Println("please provide the input file")
		os.Exit(1)
	}

	if _, err := exec.LookPath("ffmpeg"); err != nil {
		fmt.Println("ffmpeg not found in path, Intall it and try again")
		os.Exit(1)
	}

	inputPath := *inPathFlag
	var outputPath string
	if *outPathFlag == "" {
		outputPath = defaultOutputPath(*inPathFlag, format)
	} else {
		outputPath = *outPathFlag
	}

	if err := Convert(inputPath, outputPath, format); err != nil {
		fmt.Println("ffmpeg failed: ", err)
		os.Exit(1)
	}
	fmt.Println("conversion finished successfully ")
}
