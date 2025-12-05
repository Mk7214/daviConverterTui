// main.go
package main

import (
	"fmt"
	"os"
	"os/exec"
)

func main() {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		fmt.Println("ffmpeg not found in path, Intall it and try again")
	}
	var inputPath, outputPath string
	inputPath = "/home/mk14/Videos/screenrecording-2025-11-14_15-48-04.mp4"
	outputPath = "/home/mk14/Videos/newVideo.mp4"
	args := []string{
		"-y",
		"-i",
		inputPath,
		"-c:v",
		"libx264",
		"-preset",
		"medium",
		"-crf",
		"20",
		"-c:a",
		"aac",
		"-b:a",
		"192k",
		outputPath,
	}

	ffmpegCmd := exec.Command("ffmpeg", args...)
	ffmpegCmd.Stdout = os.Stdout
	ffmpegCmd.Stderr = os.Stderr
	err := ffmpegCmd.Run()
	if err != nil {
		fmt.Println("ffmpeg failed: ", err)
	} else {
		fmt.Println("conversion finished successfully ")
	}
}
