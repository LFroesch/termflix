# 🎬 TermFlix - Go Bubble Tea Edition

A sleek terminal UI video player for your personal collection, built with **Bubble Tea** and **Go**.

Features dual rendering modes: **ASCII** (works everywhere) and **Kitty graphics** (high quality).

## Features

✨ **Bubble Tea TUI:**
- Interactive list navigation
- Real-time status updates
- Beautiful styled UI

🎨 **Dual Rendering:**
- **ASCII Mode** - Works on any terminal
- **Kitty Mode** - Native image quality in Kitty terminal

📚 **Library Management:**
- Scan directories recursively
- Track watched/unwatched status
- Persistent storage in `~/.config/termflix/`
- Video duration detection

🎮 **Keyboard Controls:**
- Navigate with arrow keys
- Play with [enter]
- Delete with [d]
- Toggle mode with [m]
- Help with [?]

## Installation

### Prerequisites
- Go 1.21+
- `ffprobe` for video duration detection (usually comes with ffmpeg)

```bash
# Install ffmpeg (if not already installed)
# macOS
brew install ffmpeg

# Linux
sudo apt install ffmpeg

# or
sudo dnf install ffmpeg
```

### Build from Source

```bash
# Clone or download the files
git clone <repo> termflix
cd termflix

# Get dependencies
go mod download

# Build
go build -o termflix main.go

# Run
./termflix
```

### Or run directly

```bash
go run main.go [options]
```

## Usage

### Basic Commands

**Start the app:**
```bash
./termflix
```

**Scan a directory on startup:**
```bash
./termflix --scan ~/Movies
```

**Show help:**
```bash
./termflix --help
```

### In the TUI

| Key | Action |
|-----|--------|
| `↑`/`↓` | Navigate list |
| `Enter` | Play selected video |
| `[s]` | Scan directory |
| `[m]` | Toggle render mode (ASCII ↔ Kitty) |
| `[d]` | Delete video |
| `[?]` | Show help |
| `[q]` | Quit |

### Example Workflow

```bash
# Start and scan directory
$ ./termflix --scan ~/Videos
✓ Added 15 videos

# Now in the TUI:
# - Navigate with arrow keys
# - Press [m] to switch to Kitty graphics
# - Press [enter] to play a video
# - Press [?] to see help
```

## Architecture

```
termflix/
├── main.go          # TUI app with Bubble Tea
├── go.mod           # Dependencies
└── README.md        # This file
```

### Key Components

- **`model`** - Main Bubble Tea model with state
- **`Library`** - Video collection stored as JSON
- **`RenderMode`** - ASCII or Kitty rendering
- **`scanDirectory()`** - Recursively find videos
- **`getVideoDuration()`** - Extract duration with ffprobe

## Supported Formats

- MP4 (.mp4)
- Matroska (.mkv)
- AVI (.avi)
- MOV (.mov)
- FLV (.flv)
- WMV (.wmv)
- WebM (.webm)
- M4V (.m4v)

## Configuration

Library stored at: `~/.config/termflix/library.json`

```json
{
  "videos": [
    {
      "title": "My Video",
      "path": "/path/to/video.mp4",
      "added": "2024-01-15T10:30:00Z",
      "watched": false,
      "position": 0,
      "duration": 3600.5
    }
  ]
}
```

## Dependencies

From `go.mod`:

```
github.com/charmbracelet/bubbles       # List, progress, text input
github.com/charmbracelet/bubbletea     # TUI framework
github.com/charmbracelet/lipgloss      # Terminal styling
```

### Optional

- `ffmpeg` / `ffprobe` - For video duration detection

## Tips

1. **Large libraries?** - Scan once, list reloads instantly
2. **SSH/Remote?** - ASCII mode works perfectly over slow connections
3. **Kitty terminal?** - Switch to Kitty mode with `[m]` for stunning visuals
4. **Batch operations** - Scan multiple times to add more videos

## Future Enhancements

- [ ] Full video playback with frame rendering
- [ ] Seek/skip controls
- [ ] Playlist support
- [ ] Subtitle rendering
- [ ] Video preview thumbnails
- [ ] Search/filter by title
- [ ] Resume from last position

## Troubleshooting

**No videos found after scanning:**
```bash
# Make sure videos are in the directory
ls ~/Videos/*.mp4

# Try scanning with full path
./termflix --scan /home/user/Videos
```

**ffprobe not found:**
```bash
# Install ffmpeg
brew install ffmpeg     # macOS
sudo apt install ffmpeg # Linux
```

**Kitty mode looks wrong:**
- Your terminal doesn't support Kitty graphics protocol
- Fall back to ASCII with `[m]`
- Use native ffplay for real playback

## License

Free to use and modify!

---

**Enjoy! 🍿**

Built with ❤️ using Bubble Tea, Go, and lots of terminal magic.
