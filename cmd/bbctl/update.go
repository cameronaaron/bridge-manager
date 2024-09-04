package main

import (
    "context"
    "fmt"
    "os"
    "os/exec"
    "path"
    "strings"
    "time"

    "github.com/fatih/color"
    "github.com/urfave/cli/v2"
    "maunium.net/go/mautrix"
    "maunium.net/go/mautrix/id"

    "github.com/beeper/bridge-manager/api/hungryapi"
    "github.com/beeper/bridge-manager/log"
)

type UserError struct {
    Message string
}

func (ue UserError) Error() string {
    return ue.Message
}

var (
    Tag       string
    Commit    string
    BuildTime string

    ParsedBuildTime time.Time

    Version = "v0.12.2"
)

const BuildTimeFormat = "Jan _2 2006, 15:04:05 MST"

func init() {
    var err error
    ParsedBuildTime, err = time.Parse(time.RFC3339, BuildTime)
    if BuildTime != "" && err != nil {
        panic(fmt.Errorf("program compiled with malformed build time: %w", err))
    }
    if Tag != Version {
        if Commit == "" {
            Version = fmt.Sprintf("%s+dev.unknown", Version)
        } else {
            Version = fmt.Sprintf("%s+dev.%s", Version, Commit[:8])
        }
    }
    if BuildTime != "" {
        app.Version = fmt.Sprintf("%s (built at %s)", Version, ParsedBuildTime.Format(BuildTimeFormat))
        app.Compiled = ParsedBuildTime
    } else {
        app.Version = Version
    }
    mautrix.DefaultUserAgent = fmt.Sprintf("bbctl/%s %s", Version, mautrix.DefaultUserAgent)
}

func getDefaultConfigPath() string {
    baseConfigDir, err := os.UserConfigDir()
    if err != nil {
        panic(err)
    }
    return path.Join(baseConfigDir, "bbctl", "config.json")
}

func prepareApp(ctx *cli.Context) error {
    cfg, err := loadConfig(ctx.String("config"))
    if err != nil {
        return err
    }
    env := ctx.String("env")
    homeserver, ok := envs[env]
    if !ok {
        return fmt.Errorf("invalid environment %q", env)
    } else if err = ctx.Set("homeserver", homeserver); err != nil {
        return err
    }
    envConfig := cfg.Environments.Get(env)
    ctx.Context = context.WithValue(ctx.Context, contextKeyConfig, cfg)
    ctx.Context = context.WithValue(ctx.Context, contextKeyEnvConfig, envConfig)
    if envConfig.HasCredentials() {
        if envConfig.HungryAddress == "" || envConfig.ClusterID == "" || envConfig.Username == "" || !strings.Contains(envConfig.HungryAddress, "/_hungryserv") {
            log.Printf("Fetching whoami to fill missing env config details")
            _, err = getCachedWhoami(ctx)
            if err != nil {
                return fmt.Errorf("failed to get whoami: %w", err)
            }
        }
        homeserver := ctx.String("homeserver")
        ctx.Context = context.WithValue(ctx.Context, contextKeyMatrixClient, NewMatrixAPI(homeserver, envConfig.Username, envConfig.AccessToken))
        ctx.Context = context.WithValue(ctx.Context, contextKeyHungryClient, hungryapi.NewClient(homeserver, envConfig.HungryAddress, envConfig.Username, envConfig.AccessToken))
    }
    return nil
}

var app = &cli.App{
    Name:  "bbctl",
    Usage: "Manage self-hosted bridges for Beeper",
    Flags: []cli.Flag{
        &cli.StringFlag{
            Name:   "homeserver",
            Hidden: true,
        },
        &cli.StringFlag{
            Name:    "env",
            Aliases: []string{"e"},
            EnvVars: []string{"BEEPER_ENV"},
            Value:   "prod",
            Usage:   "The Beeper environment to connect to",
        },
        &cli.StringFlag{
            Name:    "config",
            Aliases: []string{"c"},
            EnvVars: []string{"BBCTL_CONFIG"},
            Usage:   "Path to the config file where access tokens are saved",
            Value:   getDefaultConfigPath(),
        },
        &cli.StringFlag{
            Name:    "color",
            EnvVars: []string{"BBCTL_COLOR"},
            Usage:   "Enable or disable all colors and hyperlinks in output (valid values: always/never/auto)",
            Value:   "auto",
            Action: func(ctx *cli.Context, val string) error {
                switch val {
                case "never":
                    color.NoColor = true
                case "always":
                    color.NoColor = false
                case "auto":
                    // The color package auto-detects by default
                default:
                    return fmt.Errorf("invalid value for --color: %q", val)
                }
                return nil
            },
        },
    },
    Before: prepareApp,
    Commands: []*cli.Command{
        loginCommand,
        loginPasswordCommand,
        logoutCommand,
        registerCommand,
        deleteCommand,
        whoamiCommand,
        configCommand,
        runCommand,
        proxyCommand,
        &cli.Command{
            Name:  "update",
            Usage: "Update bbctl to the latest version",
            Action: func(ctx *cli.Context) error {
                return updateBBCTL()
            },
        },
    },
}

func main() {
    if err := app.Run(os.Args); err != nil {
        _, _ = fmt.Fprintln(os.Stderr, err.Error())
    }
}

const MatrixURLTemplate = "https://matrix.%s"

func NewMatrixAPI(baseDomain string, username, accessToken string) *mautrix.Client {
    homeserverURL := fmt.Sprintf(MatrixURLTemplate, baseDomain)
    var userID id.UserID
    if username != "" {
        userID = id.NewUserID(username, baseDomain)
    }
    client, err := mautrix.NewClient(homeserverURL, userID, accessToken)
    if err != nil {
        panic(err)
    }
    return client
}

func RequiresAuth(ctx *cli.Context) error {
    if !GetEnvConfig(ctx).HasCredentials() {
        return UserError{"You're not logged in"}
    }
    return nil
}

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
    // Implement the logic to restart the bridge manager service
    // This is a placeholder implementation
    cmd := exec.Command("systemctl", "restart", "bridge-manager")
    if err := cmd.Run(); err != nil {
        return fmt.Errorf("failed to restart bridge manager: %v", err)
    }
    return nil
}