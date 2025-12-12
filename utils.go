package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
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

func formatValidator(formatFlag *string) (string, error) {
	format := strings.ToLower(*formatFlag)
	switch format {
	case "h264", "prores", "dnxhd", "wav":
		return format, nil
	default:
		errorMessage := fmt.Sprintf(
			"invalid format: %s. valid formats: h264, prores, dnxhd, wav",
			format,
		)
		return format, errors.New(errorMessage)
	}
}

func isInputEmpty(inPathFlag string) error {
	if inPathFlag == "" {
		errorMessage := fmt.Sprintln("please provide the input file")
		return errors.New(errorMessage)
	}
	return nil
}

func startConversionCmd(input, output, format string) tea.Cmd {
	return func() tea.Msg {
		ch := make(chan tea.Msg, 32)
		// probe duration
		ctx, cancel := context.WithCancel(context.Background())

		dur, err := probeDuration(input)
		if err != nil || dur <= 0 {
			return ffmpegErrMsg(fmt.Errorf("ffprobe error: %w", err))
		}

		args := buildFFmpegArgs(input, output, format)
		args = append(args, "-progress", "pipe:1", "-nostats")

		cmd := exec.CommandContext(ctx, "ffmpeg", args...)
		stdout, err := cmd.StdoutPipe()
		if err != nil {
			cancel()
			return ffmpegErrMsg(fmt.Errorf("stdout pipe error: %w", err))
		}
		stderr, err := cmd.StderrPipe()
		if err != nil {
			cancel()
			return ffmpegErrMsg(fmt.Errorf("stderr pipe error: %w", err))
		}

		if err := cmd.Start(); err != nil {
			cancel()
			return ffmpegErrMsg(fmt.Errorf("ffmpeg start failed: %w", err))
		}

		// send cmd and cancel to UI via tea.Send (we'll piggyback on a goroutine)
		go func() {
			// read stderr for occasional status lines (frame= speed= etc.)
			errScanner := bufio.NewScanner(stderr)
			for errScanner.Scan() {
				line := errScanner.Text()
				// send short status lines occasionally
				if strings.Contains(line, "frame=") || strings.Contains(line, "speed=") || strings.Contains(line, "time=") {
					select {
					case ch <- ffmpegStatusMsg(line):
					default:
					}
				}
			}
		}()
		go func() {
			sc := bufio.NewScanner(stdout)
			for sc.Scan() {
				line := sc.Text()
				// look for out_time_ms or out_time
				if strings.HasPrefix(line, "out_time_ms=") {
					val := strings.TrimPrefix(line, "out_time_ms=")
					ms, _ := strconv.ParseFloat(val, 64)
					if dur > 0 {
						percent := (ms / 1000.0) / dur
						if percent < 0 {
							percent = 0
						}
						if percent > 1 {
							percent = 1
						}
						select {
						case ch <- progressMsg(percent):
						default:
						}
					} else if strings.HasPrefix(line, "out_time=") {
						val := strings.TrimPrefix(line, "out_time=")
						// parse HH:MM:SS.xx
						if tsec, err := parseHMS(val); err == nil && dur > 0 {
							percent := tsec / dur
							if percent < 0 {
								percent = 0
							}
							if percent > 1 {
								percent = 1
							}
							select {
							case ch <- progressMsg(percent):
							default:
							}
						}
					}
				}
			}
		}()

		// goroutine: wait for command to finish and send final messages or errors
		go func() {
			err := cmd.Wait()
			cancel()
			if err != nil {
				// if cancelled by context, send cancellation error
				if ctx.Err() == context.Canceled {
					ch <- ffmpegErrMsg(fmt.Errorf("conversion canceled"))
				} else {
					ch <- ffmpegErrMsg(fmt.Errorf("ffmpeg error: %w", err))
				}
				close(ch)
				return
			}
			// success: final progress and status messages
			ch <- progressMsg(1.0)
			ch <- ffmpegStatusMsg("FINISHED_OK")
			close(ch)
		}()

		// Return a startedMsg so Update can store cancel/cmd and then begin listening.
		// We'll send the startedMsg immediately (the runtime will deliver it to Update),
		// and Update should then schedule listen(ch) to start receiving messages.
		return startedMsg{cancel: cancel, cmd: cmd, ch: ch}
	}
}

func probeDuration(path string) (float64, error) {
	// ffprobe -v error -show_entries format=duration -of default=noprint_wrappers=1:nokey=1 path
	cmd := exec.Command("ffprobe", "-v", "error", "-show_entries", "format=duration",
		"-of", "default=noprint_wrappers=1:nokey=1", path)
	out, err := cmd.Output()
	if err != nil {
		return 0, err
	}
	txt := strings.TrimSpace(string(out))
	if txt == "" {
		return 0, fmt.Errorf("ffprobe returned empty")
	}
	v, err := strconv.ParseFloat(txt, 64)
	if err != nil {
		return 0, err
	}
	return v, nil
}

func parseHMS(s string) (float64, error) {
	// "HH:MM:SS.ms"
	parts := strings.SplitN(s, ":", 3)
	if len(parts) != 3 {
		return 0, fmt.Errorf("bad time: %s", s)
	}
	h, err1 := strconv.ParseFloat(parts[0], 64)
	m, err2 := strconv.ParseFloat(parts[1], 64)
	sec, err3 := strconv.ParseFloat(parts[2], 64)
	if err1 != nil || err2 != nil || err3 != nil {
		return 0, fmt.Errorf("parse error")
	}
	return h*3600 + m*60 + sec, nil
}

func listen(ch <-chan tea.Msg) tea.Cmd {
	return func() tea.Msg {
		// blocks until a message is available
		msg, ok := <-ch
		if !ok {
			// channel closed; return nil to avoid blocking forever
			return nil
		}
		return msg
	}
}

func listenForProgress(ch chan tea.Msg) tea.Cmd {
	return func() tea.Msg {
		return <-ch // waits for a message
	}
}

func readProgress(stdout io.ReadCloser, dur float64, ch chan tea.Msg) {
	sc := bufio.NewScanner(stdout)
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(line, "out_time_ms=") {
			msString := strings.TrimPrefix(line, "out_time_ms=")
			ms, _ := strconv.ParseFloat(msString, 64)
			percent := (ms / 1_000.0) / dur
			if percent < 0 {
				percent = 0
			}
			if percent > 1 {
				percent = 1
			}

			ch <- progressMsg(percent)
		}
	}
	ch <- progressMsg(1) // final 100%
}
