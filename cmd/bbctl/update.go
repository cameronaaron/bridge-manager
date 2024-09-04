package main

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

func updateBBCTL() error {
	// Detect OS and architecture
	osType := runtime.GOOS
	arch := runtime.GOARCH

	// Determine the download URL based on OS and architecture
	var downloadURL string
	switch osType {
	case "darwin":
		switch arch {
		case "amd64":
			downloadURL = "https://nightly.link/beeper/bridge-manager/workflows/go.yaml/main/bbctl-macos-amd64.zip"
		case "arm64":
			downloadURL = "https://nightly.link/beeper/bridge-manager/workflows/go.yaml/main/bbctl-macos-arm64.zip"
		}
	case "linux":
		switch arch {
		case "amd64":
			downloadURL = "https://nightly.link/beeper/bridge-manager/workflows/go.yaml/main/bbctl-linux-amd64.zip"
		case "arm64":
			downloadURL = "https://nightly.link/beeper/bridge-manager/workflows/go.yaml/main/bbctl-linux-arm64.zip"
		}
	}

	if downloadURL == "" {
		return fmt.Errorf("unsupported OS or architecture: %s/%s", osType, arch)
	}

	// macOS-specific: Check and install Xcode Command Line Tools
	if osType == "darwin" {
		if err := checkAndInstallXcodeCLT(); err != nil {
			return err
		}

		// Check macOS version
		if err := checkMacOSVersion(); err != nil {
			fmt.Println("Warning:", err)
		}
	}

	// Backup current bbctl binary
	if err := backupCurrentBBCTL(); err != nil {
		return err
	}

	// Download and install the latest version
	if err := downloadAndInstallBBCTL(downloadURL); err != nil {
		return err
	}

	// Check permissions and functionality
	if err := checkPermissionsAndFunctionality(); err != nil {
		return err
	}

	// Auto-restart the bridge manager
	if err := autoRestartBridgeManager(); err != nil {
		return err
	}

	return nil
}

func checkAndInstallXcodeCLT() error {
	cmd := exec.Command("xcode-select", "--install")
	output, err := cmd.CombinedOutput()
	if err != nil && !strings.Contains(string(output), "already installed") {
		return fmt.Errorf("failed to install Xcode Command Line Tools: %v", err)
	}
	return nil
}

func checkMacOSVersion() error {
	cmd := exec.Command("sw_vers", "-productVersion")
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("failed to check macOS version: %v", err)
	}

	version := strings.TrimSpace(string(output))
	if strings.Compare(version, "12.0.0") < 0 {
		return fmt.Errorf("macOS version is older than Ventura (12.0.0). Please consider upgrading.")
	}
	return nil
}

func backupCurrentBBCTL() error {
	currentPath, err := exec.LookPath("bbctl")
	if err != nil {
		return fmt.Errorf("failed to locate current bbctl binary: %v", err)
	}

	backupPath := os.Getenv("HOME") + "/bbctl.bak"
	if err := os.Rename(currentPath, backupPath); err != nil {
		return fmt.Errorf("failed to backup current bbctl binary: %v", err)
	}
	return nil
}

func downloadAndInstallBBCTL(downloadURL string) error {
	cmd := exec.Command("curl", "-L", "-o", "/usr/local/bin/bbctl", downloadURL)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to download and install bbctl: %v", err)
	}

	if err := os.Chmod("/usr/local/bin/bbctl", 0755); err != nil {
		return fmt.Errorf("failed to set permissions for bbctl: %v", err)
	}
	return nil
}

func checkPermissionsAndFunctionality() error {
	cmd := exec.Command("bbctl", "--version")
	if err := cmd.Run(); err != nil {
		if err := os.Chmod("/usr/local/bin/bbctl", 0755); err != nil {
			return fmt.Errorf("failed to set permissions for bbctl: %v", err)
		}
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("bbctl is not functioning correctly after update: %v", err)
		}
	}
	return nil
}

func autoRestartBridgeManager() error {
	// Placeholder for auto-restart logic
	return nil
}
