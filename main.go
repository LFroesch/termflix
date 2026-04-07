package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/textinput"
	bubbletea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Video structure
type Video struct {
	Title    string    `json:"title"`
	Path     string    `json:"path"`
	Added    time.Time `json:"added"`
	Watched  bool      `json:"watched"`
	Position float64   `json:"position"`
	Duration float64   `json:"duration"`
}

// Library structure
type Library struct {
	Videos []Video `json:"videos"`
}

// Styles
var (
	titleStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("229")).
			Background(lipgloss.Color("57")).
			Padding(0, 1)
)

// Video item for list
type VideoItem struct {
	video Video
	index int
}

func (i VideoItem) FilterValue() string { return i.video.Title }

func (i VideoItem) Title() string {
	status := "○"
	if i.video.Watched {
		status = "✓"
	}
	return fmt.Sprintf("[%d] %s %s", i.index+1, status, i.video.Title)
}

func (i VideoItem) Description() string {
	dur := formatDuration(i.video.Duration)
	if dur == "00:00" {
		dur = "unknown"
	}
	added := i.video.Added.Format("Jan 02")
	return fmt.Sprintf("   %s • Added %s", dur, added)
}

// Package-level playback handles — single-user TUI, only one active at a time.
var (
	playbackFFmpeg *exec.Cmd
	playbackAudio  *exec.Cmd
)

func stopPlayback() {
	if playbackFFmpeg != nil && playbackFFmpeg.Process != nil {
		playbackFFmpeg.Process.Kill()
		playbackFFmpeg = nil
	}
	if playbackAudio != nil && playbackAudio.Process != nil {
		playbackAudio.Process.Kill()
		playbackAudio = nil
	}
}

// Main model
type model struct {
	list         list.Model
	library      Library
	currentView  string // "menu", "playing", "scanning", "help"
	selectedIdx  int
	textInput    textinput.Model
	progress     progress.Model
	statusMsg    string
	width        int
	height       int
	currentFrame string
}

func initialModel() model {
	items := []list.Item{}
	lib := loadLibrary()

	for i, v := range lib.Videos {
		items = append(items, VideoItem{video: v, index: i})
	}

	l := list.New(items, list.NewDefaultDelegate(), 0, 0)
	l.Title = ""
	l.SetShowTitle(false)
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(false)
	l.SetShowHelp(false)

	ti := textinput.New()
	ti.Placeholder = "Enter path..."
	ti.CharLimit = 256

	p := progress.New(
		progress.WithDefaultGradient(),
		progress.WithWidth(40),
		progress.WithoutPercentage(),
	)

	return model{
		list:        l,
		library:     lib,
		currentView: "menu",
		textInput:   ti,
		progress:    p,
		statusMsg:   "Welcome to TermFlix! Press [?] for help",
	}
}

func (m model) Init() bubbletea.Cmd {
	return nil
}

// Messages
type videoDurationMsg struct {
	index    int
	duration float64
	err      error
}

type videoPlaybackMsg struct {
	video Video
	err   error
}

type scanCompleteMsg struct {
	videos []Video
	added  int
	err    error
}

// frameMsg carries one rendered frame and the command to read the next one.
type frameMsg struct {
	frame string
	next  bubbletea.Cmd
}

// Update
func (m model) Update(msg bubbletea.Msg) (bubbletea.Model, bubbletea.Cmd) {
	switch msg := msg.(type) {
	case bubbletea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			if m.currentView == "playing" {
				stopPlayback()
				m.currentView = "menu"
				m.currentFrame = ""
				m.statusMsg = "Stopped"
				return m, nil
			}
			if m.currentView == "menu" {
				return m, bubbletea.Quit
			}
			m.currentView = "menu"
			m.statusMsg = "Cancelled"
			return m, nil

		case "?":
			if m.currentView == "menu" {
				m.currentView = "help"
				return m, nil
			}

		case "enter":
			if m.currentView == "menu" && len(m.list.Items()) > 0 {
				m.selectedIdx = m.list.Index()
				m.currentView = "playing"
				m.currentFrame = ""
				m.statusMsg = "Loading..."
				return m, m.playVideo(m.selectedIdx)
			} else if m.currentView == "scanning" {
				path := strings.TrimSpace(m.textInput.Value())
				if path == "" {
					m.currentView = "menu"
					m.statusMsg = "Scan cancelled"
					return m, nil
				}
				m.currentView = "menu"
				return m, scanDirectoryCmd(path)
			} else if m.currentView == "help" {
				m.currentView = "menu"
				return m, nil
			}

		case "s":
			if m.currentView == "menu" {
				m.currentView = "scanning"
				m.textInput.Focus()
				m.statusMsg = "Enter directory path to scan..."
				return m, nil
			}

		case "delete", "d":
			if m.currentView == "menu" && len(m.list.Items()) > 0 {
				idx := m.list.Index()
				m.library.Videos = append(
					m.library.Videos[:idx],
					m.library.Videos[idx+1:]...,
				)
				saveLibrary(m.library)
				m.list.RemoveItem(idx)
				m.statusMsg = fmt.Sprintf("Removed video (Total: %d)", len(m.library.Videos))
			}
		}

	case bubbletea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		// header(1) + sep(1) + sep(1) + footer(1) = 4 lines overhead
		m.list.SetSize(msg.Width-2, msg.Height-4)

	case frameMsg:
		if m.currentView == "playing" {
			m.currentFrame = msg.frame
			return m, msg.next
		}
		// View changed (user pressed q), don't chain next frame
		return m, nil

	case videoDurationMsg:
		if msg.err == nil && msg.index < len(m.library.Videos) {
			m.library.Videos[msg.index].Duration = msg.duration
			saveLibrary(m.library)
			m.list.SetItem(msg.index, VideoItem{
				video: m.library.Videos[msg.index],
				index: msg.index,
			})
		}

	case videoPlaybackMsg:
		// ffmpeg finished (end of video or error)
		stopPlayback()
		m.currentView = "menu"
		m.currentFrame = ""
		if msg.err != nil {
			m.statusMsg = fmt.Sprintf("Error: %v", msg.err)
		} else {
			m.statusMsg = fmt.Sprintf("Finished: %s", msg.video.Title)
			if m.selectedIdx < len(m.library.Videos) {
				m.library.Videos[m.selectedIdx].Watched = true
				saveLibrary(m.library)
				m.list.SetItem(m.selectedIdx, VideoItem{
					video: m.library.Videos[m.selectedIdx],
					index: m.selectedIdx,
				})
			}
		}

	case scanCompleteMsg:
		if msg.err != nil {
			m.statusMsg = fmt.Sprintf("Scan error: %v", msg.err)
		} else {
			m.library.Videos = append(m.library.Videos, msg.videos...)
			seen := make(map[string]bool)
			var unique []Video
			for _, v := range m.library.Videos {
				if !seen[v.Path] {
					unique = append(unique, v)
					seen[v.Path] = true
				}
			}
			m.library.Videos = unique
			saveLibrary(m.library)
			items := make([]list.Item, len(m.library.Videos))
			for i, v := range m.library.Videos {
				items[i] = VideoItem{video: v, index: i}
			}
			m.list.SetItems(items)
			m.statusMsg = fmt.Sprintf("✓ Added %d videos (Total: %d)", msg.added, len(m.library.Videos))
		}
	}

	// Handle input based on current view
	if m.currentView == "scanning" {
		m.textInput, _ = m.textInput.Update(msg)
	} else if m.currentView == "menu" {
		m.list, _ = m.list.Update(msg)
	}

	return m, nil
}


// View
func (m model) View() string {
	switch m.currentView {
	case "help":
		return m.viewHelp()
	case "scanning":
		return m.viewScan()
	case "playing":
		return m.viewPlaying()
	default:
		return m.viewMenu()
	}
}

func (m model) renderHeader() string {
	left := titleStyle.Render("termflix")

	watched := 0
	for _, v := range m.library.Videos {
		if v.Watched {
			watched++
		}
	}
	right := lipgloss.NewStyle().Faint(true).Render(
		fmt.Sprintf("%d videos · %d watched", len(m.library.Videos), watched),
	)

	gap := m.width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		gap = 1
	}
	return left + strings.Repeat(" ", gap) + right
}

func (m model) renderFooter() string {
	keys := lipgloss.NewStyle().Faint(true).Render("[enter] play  [s] scan  [d] delete  [?] help  [q] quit")
	status := lipgloss.NewStyle().Foreground(lipgloss.Color("229")).Render(m.statusMsg)
	gap := m.width - lipgloss.Width(keys) - lipgloss.Width(status) - 2
	if gap < 1 {
		gap = 1
	}
	return " " + keys + strings.Repeat(" ", gap) + status
}

func (m model) sep() string {
	return lipgloss.NewStyle().Faint(true).Render(strings.Repeat("─", m.width))
}

func (m model) viewMenu() string {
	if m.width == 0 {
		return "loading..."
	}
	return strings.Join([]string{
		m.renderHeader(),
		m.sep(),
		m.list.View(),
		m.sep(),
		m.renderFooter(),
	}, "\n")
}

func (m model) viewScan() string {
	if m.width == 0 {
		return "loading..."
	}
	content := "\n  Scan Directory\n\n  Enter path to scan for videos:\n\n  " +
		m.textInput.View() + "\n\n  " +
		lipgloss.NewStyle().Faint(true).Render("(Press enter to start scanning, or [q] to cancel)")
	footer := lipgloss.NewStyle().Faint(true).Render(" [enter] confirm  [q] cancel")

	return strings.Join([]string{m.renderHeader(), m.sep(), content, m.sep(), footer}, "\n")
}

func (m model) viewPlaying() string {
	if m.width == 0 {
		return "loading..."
	}

	contentH := m.height - 5 // header+sep+sep+footer+cursor = 5 reserved
	var content string

	if m.currentFrame == "" {
		// Loading placeholder
		padding := contentH / 2
		content = strings.Repeat("\n", padding) +
			lipgloss.NewStyle().Faint(true).Render("  Loading video...")
	} else {
		// Trim/pad frame to fit content area exactly
		lines := strings.Split(m.currentFrame, "\n")
		if len(lines) > contentH {
			lines = lines[:contentH]
		}
		for len(lines) < contentH {
			lines = append(lines, "")
		}
		content = strings.Join(lines, "\n")
	}

	title := ""
	if m.selectedIdx < len(m.library.Videos) {
		title = m.library.Videos[m.selectedIdx].Title
	}
	footer := lipgloss.NewStyle().Faint(true).Render(fmt.Sprintf(" [q] stop  |  %s", title))

	return strings.Join([]string{m.renderHeader(), m.sep(), content, m.sep(), footer}, "\n")
}

func (m model) viewHelp() string {
	if m.width == 0 {
		return "loading..."
	}

	watched := 0
	for _, v := range m.library.Videos {
		if v.Watched {
			watched++
		}
	}
	content := fmt.Sprintf(`
  KEYBINDINGS
  [enter] play  [s] scan  [d] delete  [?] help  [q] quit/stop

  PLAYBACK
  Video renders inside the content area — header and footer always visible.
  Audio plays via mpv (--no-video). Press [q] to stop.

  STATS
  Total: %d  Watched: %d`, len(m.library.Videos), watched)

	footer := lipgloss.NewStyle().Faint(true).Render(" [enter] or [q] back to menu")
	return strings.Join([]string{m.renderHeader(), m.sep(), content, m.sep(), footer}, "\n")
}

// Library functions
func loadLibrary() Library {
	configDir := filepath.Join(os.Getenv("HOME"), ".config", "termflix")
	os.MkdirAll(configDir, 0755)

	libPath := filepath.Join(configDir, "library.json")
	data, err := os.ReadFile(libPath)
	if err != nil {
		return Library{Videos: []Video{}}
	}

	var lib Library
	json.Unmarshal(data, &lib)
	return lib
}

func saveLibrary(lib Library) error {
	configDir := filepath.Join(os.Getenv("HOME"), ".config", "termflix")
	os.MkdirAll(configDir, 0755)

	libPath := filepath.Join(configDir, "library.json")
	data, err := json.MarshalIndent(lib, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(libPath, data, 0644)
}

// Command functions
func scanDirectoryCmd(path string) bubbletea.Cmd {
	return func() bubbletea.Msg {
		videos, err := scanDirectory(path)
		if err != nil {
			return scanCompleteMsg{err: err}
		}
		return scanCompleteMsg{videos: videos, added: len(videos)}
	}
}

// playVideo starts ffmpeg piping raw frames and mpv for audio.
// Frames are delivered to bubbletea one at a time via frameMsg.
func (m model) playVideo(idx int) bubbletea.Cmd {
	if idx >= len(m.library.Videos) {
		return func() bubbletea.Msg {
			return videoPlaybackMsg{err: fmt.Errorf("invalid video index")}
		}
	}

	video := m.library.Videos[idx]

	if _, err := os.Stat(video.Path); err != nil {
		return func() bubbletea.Msg {
			return videoPlaybackMsg{err: fmt.Errorf("file not found: %s", video.Path)}
		}
	}

	if _, err := exec.LookPath("ffmpeg"); err != nil {
		return func() bubbletea.Msg {
			return videoPlaybackMsg{err: fmt.Errorf("ffmpeg not found in PATH")}
		}
	}

	// Content area dimensions — alt screen reserves one extra row for cursor
	pixW := m.width
	contentH := m.height - 5 // header+sep+sep+footer+cursor = 5 reserved
	if contentH < 2 {
		contentH = 2
	}
	pixH := contentH * 2 // half-block: 2 pixel rows per terminal row
	// ensure even dimensions
	pixW = (pixW / 2) * 2
	pixH = (pixH / 2) * 2
	if pixW <= 0 || pixH <= 0 {
		pixW, pixH = 80, 48
	}

	// Log ffmpeg stderr so we can debug issues
	logFile, _ := os.OpenFile("/tmp/termflix-ffmpeg.log", os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)

	// ffmpeg: decode video to raw RGB frames, scaled to content area
	ffmpegArgs := []string{
		"-i", video.Path,
		"-vf", fmt.Sprintf("fps=15,scale=%d:%d", pixW, pixH),
		"-f", "rawvideo",
		"-pix_fmt", "rgb24",
		"-an",
		"pipe:1",
	}
	ffCmd := exec.Command("ffmpeg", ffmpegArgs...)
	pipe, err := ffCmd.StdoutPipe()
	if err != nil {
		return func() bubbletea.Msg {
			return videoPlaybackMsg{err: fmt.Errorf("ffmpeg pipe: %v", err)}
		}
	}
	if logFile != nil {
		ffCmd.Stderr = logFile
	} else {
		ffCmd.Stderr = io.Discard
	}

	if err := ffCmd.Start(); err != nil {
		return func() bubbletea.Msg {
			return videoPlaybackMsg{err: fmt.Errorf("ffmpeg start: %v", err)}
		}
	}
	playbackFFmpeg = ffCmd

	// Audio via mpv --no-video
	if _, err := exec.LookPath("mpv"); err == nil {
		audCmd := exec.Command("mpv", "--no-video", "--really-quiet", video.Path)
		audCmd.Stdout = io.Discard
		audCmd.Stderr = io.Discard
		audCmd.Start()
		playbackAudio = audCmd
	}

	frameSize := pixW * pixH * 3
	return readFrameCmd(ffCmd, pipe, frameSize, pixW, pixH, video)
}

const targetFPS = 15
const frameDuration = time.Second / targetFPS

// readFrameCmd reads one raw RGB frame from the pipe, renders it, and returns
// a frameMsg containing the rendered string plus the command for the next frame.
// It throttles to targetFPS by sleeping for any remaining time in the frame interval.
func readFrameCmd(cmd *exec.Cmd, pipe io.ReadCloser, frameSize, pixW, pixH int, video Video) bubbletea.Cmd {
	return func() bubbletea.Msg {
		frameStart := time.Now()

		buf := make([]byte, frameSize)
		_, err := io.ReadFull(pipe, buf)
		if err != nil {
			pipe.Close()
			go cmd.Wait()
			return videoPlaybackMsg{video: video}
		}

		frame := renderHalfBlock(buf, pixW, pixH)

		// Throttle: sleep for remaining frame time so we render at ~targetFPS
		elapsed := time.Since(frameStart)
		if elapsed < frameDuration {
			time.Sleep(frameDuration - elapsed)
		}

		next := readFrameCmd(cmd, pipe, frameSize, pixW, pixH, video)
		return frameMsg{frame: frame, next: next}
	}
}

// renderHalfBlock converts a raw RGB pixel buffer to a string using half-block
// characters (▀) with ANSI true-color. Upper pixel = fg, lower pixel = bg.
// Each terminal row displays two rows of pixels, doubling effective resolution.
func renderHalfBlock(data []byte, width, height int) string {
	var sb strings.Builder

	for y := 0; y+1 < height; y += 2 {
		for x := 0; x < width; x++ {
			topIdx := (y*width + x) * 3
			botIdx := ((y+1)*width + x) * 3

			if topIdx+2 >= len(data) || botIdx+2 >= len(data) {
				break
			}

			tr, tg, tb := data[topIdx], data[topIdx+1], data[topIdx+2]
			br, bg, bb := data[botIdx], data[botIdx+1], data[botIdx+2]

			// Set fg (upper pixel) and bg (lower pixel), then draw ▀
			fmt.Fprintf(&sb, "\x1b[38;2;%d;%d;%dm\x1b[48;2;%d;%d;%dm▀", tr, tg, tb, br, bg, bb)
		}
		sb.WriteString("\x1b[0m\n")
	}

	return sb.String()
}

// Utility functions
func formatDuration(seconds float64) string {
	if seconds <= 0 {
		return "00:00"
	}

	hrs := int(seconds) / 3600
	mins := (int(seconds) % 3600) / 60
	secs := int(seconds) % 60

	if hrs > 0 {
		return fmt.Sprintf("%02d:%02d:%02d", hrs, mins, secs)
	}
	return fmt.Sprintf("%02d:%02d", mins, secs)
}

func getVideoDuration(videoPath string) (float64, error) {
	cmd := exec.Command("ffprobe",
		"-v", "error",
		"-show_entries", "format=duration",
		"-of", "default=noprint_wrappers=1:nokey=1",
		videoPath,
	)

	output, err := cmd.Output()
	if err != nil {
		return 0, err
	}

	return strconv.ParseFloat(strings.TrimSpace(string(output)), 64)
}

func expandPath(path string) string {
	if strings.HasPrefix(path, "~/") {
		return filepath.Join(os.Getenv("HOME"), path[2:])
	}
	return path
}

func scanDirectory(path string) ([]Video, error) {
	path = expandPath(path)
	var videos []Video

	supportedExts := map[string]bool{
		".mp4": true, ".mkv": true, ".avi": true, ".mov": true,
		".flv": true, ".wmv": true, ".webm": true, ".m4v": true,
	}

	err := filepath.Walk(path, func(filePath string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}

		ext := strings.ToLower(filepath.Ext(filePath))
		if !supportedExts[ext] {
			return nil
		}

		duration, _ := getVideoDuration(filePath)

		videos = append(videos, Video{
			Title:    strings.TrimSuffix(filepath.Base(filePath), ext),
			Path:     filePath,
			Added:    time.Now(),
			Duration: duration,
		})
		return nil
	})

	sort.Slice(videos, func(i, j int) bool {
		return videos[i].Title < videos[j].Title
	})

	return videos, err
}

// Main
func main() {
	scanPath := ""
	for i := 0; i < len(os.Args)-1; i++ {
		if os.Args[i] == "--scan" {
			scanPath = os.Args[i+1]
		}
		if os.Args[i] == "--help" {
			fmt.Println(`TermFlix - Terminal Video Player

Usage:
  termflix [--scan <path>] [--help]

Requires: ffmpeg, ffprobe, mpv (for audio)`)
			os.Exit(0)
		}
	}

	lib := loadLibrary()
	if scanPath != "" {
		videos, err := scanDirectory(scanPath)
		if err == nil && len(videos) > 0 {
			lib.Videos = append(lib.Videos, videos...)
			saveLibrary(lib)
			fmt.Printf("✓ Added %d videos\n", len(videos))
		}
	}

	p := bubbletea.NewProgram(initialModel(), bubbletea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
