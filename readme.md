# PingChecker

**PingChecker** is a lightweight, standalone utility to monitor the real-time network latency (ping) of a specific (game) process.

<img width="493" height="276" alt="image" src="https://github.com/user-attachments/assets/435121e8-0422-464e-a5c6-1944b29a36db" />

It uses **Fyne** for a native, GPU-accelerated UI and **Raw ICMP Sockets** for accurate network measurement.

## 🚀 Features
*   **Process Isolation:** Automatically detects the public IP address a specific game process is communicating with.
*   **Smart Filtering:** Automatically hides system processes (`svchost`, `csrss`) and prioritizes games (Steam, Riot, Epic).
*   **Zero-Overhead:** Uses negligible CPU/RAM.
*   **Real-Time:** Updates latency every second.

---

## 🛠️ Project Structure

```text
PingChecker/
├── cmd/
│   └── pingchecker/
│       └── main.go      # Entry point and core logic
├── go.mod               # Dependency definitions
├── go.sum               # Checksums for dependencies
└── README.md            # This file
```

---

## ⚙️ Prerequisites

Before running or building, ensure you have the following installed:

1.  **Go (Golang):** [Download here](https://go.dev/dl/).
2.  **C Compiler (GCC):** Required for the Fyne UI toolkit.
    *   **Windows:** Install [TDM-GCC](https://jmeubank.github.io/tdm-gcc/).
    *   **Linux:** `sudo apt install gcc libgl1-mesa-dev xorg-dev`
    *   **Mac:** `xcode-select --install`
3.  **VS Code:** With the official "Go" extension installed.

---

## 💻 Setup & Installation (VS Code)

1.  **Open the Project:**
    Open VS Code, go to `File > Open Folder`, and select `Documents/PingChecker`.

2.  **Initialize Dependencies:**
    Open the Terminal (`Ctrl + ~`) and run:
    ```bash
    go mod tidy
    ```
    *This downloads `fyne`, `gopsutil`, and `pro-bing`.*

3.  **Verify Installation:**
    Ensure no red squiggly lines appear in `main.go`.

4. **Run locally before building:**
    ```bash
    go run ./cmd/pingchecker
    ```

---

## 🏗️ Build Instructions

To create a standalone `.exe` file that hides the command prompt window:

1.  Open the Terminal in VS Code.
2.  Run the following command:

```bash
go build -ldflags "-H=windowsgui" -o PingChecker.exe ./cmd/pingchecker
```

**Flag Explanations:**
*   `-H=windowsgui`: Prevents a black console window from appearing behind the app.
*   `-o PingChecker.exe`: Sets the output filename.
*   `./cmd/pingchecker`: Tells the compiler where the `main.go` file is located.

*Note: Removed `-s -w` (strip symbols) to reduce false positives from Windows Defender.* Previously (go build -ldflags "-H=windowsgui -s -w" -o PingChecker.exe)

---

## ⚠️ Critical Runtime Requirements

### 1. Administrator Privileges
Because this tool uses **Raw ICMP Sockets** (Ping) and reads **Process Network Tables**, Windows requires elevated permissions.

*   **Development:** You must run VS Code as Administrator to debug.
*   **Production:** You must right-click `PingChecker.exe` and select **"Run as Administrator"**.

*If you do not run as Admin, the app will open, but the ping will fail or show "Error".*

### 2. Antivirus / Windows Defender
Because this is an unsigned tool that scans processes and sends network packets, Windows Defender may flag it as `Behavior:Win32/DefenseEvasion`.

**Solution:**
1.  Open **Windows Security**.
2.  Go to **Virus & threat protection > Manage settings > Exclusions**.
3.  Add the `PingChecker` folder to the exclusion list.

---

## 🐞 Troubleshooting

**Q: The app crashes immediately upon opening.**
A: You likely don't have a C Compiler (GCC) installed, or your graphics drivers are outdated (Fyne requires OpenGL).

**Q: It says "Pinger Error: socket: permission denied".**
A: You are not running as Administrator.

**Q: The process list is empty.**

A: Ensure you are running as Administrator. Standard users cannot read the paths of processes started by other users or the system.
