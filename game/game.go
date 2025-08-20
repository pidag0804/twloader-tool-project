// twloader-tool/game/game.go
package game

import (
	"bufio"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/sys/windows/registry"
)

const (
	auditionRegistryPath = `SOFTWARE\Wow6432Node\HappyTuk\Audition`
	gamePatchInfoURL     = "http://auditionpatch.mangot5.com//audition_patch/patch/live/audition/package/PackageInfo.txt"
)

type UpdateInfo struct {
	UpdateNeeded bool   `json:"updateNeeded"`
	PatcherPath  string `json:"-"` // Don't expose path to client
	Error        string `json:"error,omitempty"`
}

var (
	updateState      UpdateInfo
	updateStateMutex = &sync.RWMutex{}
	logger           = log.New(os.Stdout, "GAME | ", log.LstdFlags)
)

func CheckVersion() {
	if runtime.GOOS != "windows" {
		return
	}
	logger.Println("Checking for Audition game updates...")

	localVersion, installPath, err := getLocalGameInfo()
	if err != nil {
		logger.Printf("Could not get local game info: %v. Skipping update check.", err)
		return
	}
	logger.Printf("Found local game version: %d at %s", localVersion, installPath)

	remoteVersion, err := getRemoteGameVersion()
	if err != nil {
		logger.Printf("Could not get remote game version: %v. Skipping update check.", err)
		return
	}
	logger.Printf("Found remote server version: %d", remoteVersion)

	updateStateMutex.Lock()
	defer updateStateMutex.Unlock()

	if localVersion < remoteVersion {
		logger.Println("Local version is outdated. Update is available.")
		patcherPath := filepath.Join(installPath, "patcher.exe")
		if _, err := os.Stat(patcherPath); err == nil {
			updateState.UpdateNeeded = true
			updateState.PatcherPath = patcherPath
		} else {
			updateState.UpdateNeeded = false
			updateState.Error = "Update needed, but patcher.exe not found."
			logger.Printf("Patcher not found at %s", patcherPath)
		}
	} else {
		logger.Println("Game is up to date.")
		updateState.UpdateNeeded = false
	}
}

func getLocalGameInfo() (version int, installPath string, err error) {
	key, err := registry.OpenKey(registry.LOCAL_MACHINE, auditionRegistryPath, registry.QUERY_VALUE)
	if err != nil {
		return 0, "", fmt.Errorf("could not open registry key: %w", err)
	}
	defer key.Close()

	ver, _, err := key.GetIntegerValue("VERSION")
	if err != nil {
		return 0, "", fmt.Errorf("could not read 'VERSION' from registry: %w", err)
	}

	path, _, err := key.GetStringValue("installpath")
	if err != nil {
		return 0, "", fmt.Errorf("could not read 'installpath' from registry: %w", err)
	}
	if path == "" {
		return 0, "", fmt.Errorf("'installpath' value is empty")
	}

	return int(ver), path, nil
}

func getRemoteGameVersion() (version int, err error) {
	client := http.Client{Timeout: 15 * time.Second}
	resp, err := client.Get(gamePatchInfoURL)
	if err != nil {
		return 0, fmt.Errorf("could not fetch patch info: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("server returned non-200 status: %s", resp.Status)
	}

	scanner := bufio.NewScanner(resp.Body)
	if scanner.Scan() {
		line := scanner.Text()
		parts := strings.Split(line, ",")
		if len(parts) == 2 && strings.ToUpper(parts[0]) == "VERSION" {
			version, err := strconv.Atoi(strings.TrimSpace(parts[1]))
			if err != nil {
				return 0, fmt.Errorf("could not parse version number '%s': %w", parts[1], err)
			}
			return version, nil
		}
		return 0, fmt.Errorf("unexpected format in first line: %s", line)
	}
	if err := scanner.Err(); err != nil {
		return 0, fmt.Errorf("error reading response body: %w", err)
	}
	return 0, fmt.Errorf("patch info file is empty or unreadable")
}

func GetInstallPath() (string, error) {
	if runtime.GOOS != "windows" {
		return "", fmt.Errorf("此功能僅適用於 Windows")
	}
	key, err := registry.OpenKey(registry.LOCAL_MACHINE, auditionRegistryPath, registry.QUERY_VALUE)
	if err != nil {
		return "", fmt.Errorf("無法開啟登錄檔機碼: %w", err)
	}
	defer key.Close()

	installPath, _, err := key.GetStringValue("installpath")
	if err != nil {
		return "", fmt.Errorf("無法讀取 'installpath' 值: %w", err)
	}
	if installPath == "" {
		return "", fmt.Errorf("'installpath' 值為空")
	}

	return installPath, nil
}

func SetupGamePathLink() {
	if runtime.GOOS != "windows" {
		return
	}
	logger.Println("Setting up game path link...")

	key, err := registry.OpenKey(registry.LOCAL_MACHINE, `SOFTWARE\Wow6432Node\HappyTuk\Audition`, registry.QUERY_VALUE)
	if err != nil {
		log.Printf("Could not open registry key (Audition may not be installed): %v", err)
		return
	}
	defer key.Close()

	installPath, _, err := key.GetStringValue("installpath")
	if err != nil {
		log.Printf("Could not read 'installpath' value from registry: %v", err)
		return
	}
	executeName, _, err := key.GetStringValue("EXECUTE")
	if err != nil {
		log.Printf("Could not read 'EXECUTE' value from registry: %v", err)
		return
	}
	if installPath == "" || executeName == "" {
		log.Println("Registry values for 'installpath' or 'EXECUTE' are empty. Aborting setup.")
		return
	}

	fullGamePath := filepath.Join(installPath, executeName)
	log.Printf("Successfully determined game executable path: %s", fullGamePath)

	targetDirs := []string{
		filepath.Join(DefaultBaseDir, "Plus"),
		filepath.Join(DefaultBaseDir, "PlusUP"),
	}

	for _, dir := range targetDirs {
		if _, err := os.Stat(dir); !os.IsNotExist(err) {
			savePrevPath := filepath.Join(dir, "SavePrev.txt")
			err := os.WriteFile(savePrevPath, []byte(fullGamePath), 0666)
			if err != nil {
				log.Printf("Error: Failed to write to file %s: %v", savePrevPath, err)
			} else {
				log.Printf("Successfully wrote game path to %s", savePrevPath)
			}
		} else {
			log.Printf("Directory does not exist, skipping: %s", dir)
		}
	}
}

func GetUpdateState() UpdateInfo {
	updateStateMutex.RLock()
	defer updateStateMutex.RUnlock()
	return updateState
}

func RunPatcher() error {
	updateStateMutex.RLock()
	needsUpdate := updateState.UpdateNeeded
	patcherPath := updateState.PatcherPath
	updateStateMutex.RUnlock()

	if !needsUpdate || patcherPath == "" {
		return fmt.Errorf("no update is currently required or patcher path is unknown")
	}

	logger.Printf("Received request to launch patcher: %s", patcherPath)
	cmd := exec.Command(patcherPath)
	cmd.Dir = filepath.Dir(patcherPath)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to launch patcher.exe: %w", err)
	}

	go func() {
		time.Sleep(5 * time.Second)
		updateStateMutex.Lock()
		updateState.UpdateNeeded = false
		updateStateMutex.Unlock()
		logger.Println("Resetting update needed state after launching patcher.")
	}()

	return nil
}

func Launch(mode string) error {
	if mode != "plus" && mode != "plusup" {
		return fmt.Errorf("無效的啟動模式: %s", mode)
	}

	basePath, err := ResolveBasePath()
	if err != nil {
		return err
	}

	var subDir string
	if mode == "plus" {
		subDir = "Plus"
	} else {
		subDir = "PlusUP"
	}

	exePath := filepath.Join(basePath, subDir, "TWLoader.exe")
	logger.Printf("嘗試啟動: %s", exePath)

	if _, err := os.Stat(exePath); os.IsNotExist(err) {
		return fmt.Errorf("找不到執行檔: %s", exePath)
	}

	cmd := exec.Command(exePath)
	cmd.Dir = filepath.Dir(exePath)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("啟動程式失敗: %w", err)
	}

	logger.Printf("成功發出啟動命令: %s", exePath)
	return nil
}
