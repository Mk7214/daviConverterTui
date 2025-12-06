package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func Convert(inputPath, outputPath, format string) error {
	args := buildFFmpegArgs(inputPath, outputPath, format)

	ffmpegCmd := exec.Command("ffmpeg", args...)
	ffmpegCmd.Stdout = os.Stdout
	ffmpegCmd.Stderr = os.Stderr

	return ffmpegCmd.Run()
}

func buildFFmpegArgs(inputPath, outputPath, format string) []string {
	switch format {
	case "h264":
		return []string{
			"-y",
			"-i", inputPath,
			"-c:v", "libx264",
			"-preset", "medium",
			"-crf", "20",
			"-c:a", "aac",
			"-b:a", "192k",
			outputPath,
		}
	case "prores":
		// ProRes HQ
		return []string{
			"-y",
			"-i", inputPath,
			"-c:v", "prores_ks",
			"-profile:v", "3",
			"-pix_fmt", "yuv422p10le",
			"-c:a", "pcm_s16le",
			outputPath,
		}
	case "dnxhd":
		// Basic DNxHD example for 1080p
		return []string{
			"-y",
			"-i", inputPath,
			"-c:v", "dnxhd",
			"-b:v", "185M",
			"-pix_fmt", "yuv422p",
			"-c:a", "pcm_s16le",
			outputPath,
		}
	case "wav":
		// audio-only WAV 48kHz stereo
		return []string{
			"-y",
			"-i", inputPath,
			"-vn",
			"-ar", "48000",
			"-ac", "2",
			"-c:a", "pcm_s16le",
			outputPath,
		}
	default:
		// fallback to h264
		return []string{
			"-y",
			"-i", inputPath,
			"-c:v", "libx264",
			"-preset", "medium",
			"-crf", "20",
			"-c:a", "aac",
			"-b:a", "192k",
			outputPath,
		}
	}
}

func defaultOutputPath(input string, format string) string {
	path := filepath.Dir(input)           // gets the path like /home/videos from the input without the filename
	file := filepath.Base(input)          // get the filename with extension
	ext := filepath.Ext(file)             // gets the extension of the file
	base := strings.TrimSuffix(file, ext) // removes the extenstion

	var newName string
	switch format {
	case "h264":
		newName = base + "_h264.mp4"
	case "prores":
		newName = base + "_prores.mov"
	case "dnxhd":
		newName = base + "_dnx.mxf"
	case "wav":
		newName = base + "_48k.wav"
	default:
		newName = base + "_out.mp4"
	}
	return filepath.Join(path, newName)
}
