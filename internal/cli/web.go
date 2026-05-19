package cli

import (
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"runtime"

	"github.com/spf13/cobra"

	"github.com/diasYuri/agentflow/internal/web"
	"github.com/diasYuri/agentflow/internal/web/settings"
)

// webFlags backs the `agentflow web` command. Unset values stay as the
// zero pointers so the settings package can distinguish "user did not
// pass this flag" from "user pinned the default".
type webFlags struct {
	host      string
	port      int
	noOpen    bool
	devAssets string
	daemon    string
	root      string
	token     string

	hostSet      bool
	portSet      bool
	devAssetsSet bool
	daemonSet    bool
	rootSet      bool
	tokenSet     bool
}

func newWebCommand() *cobra.Command {
	flags := &webFlags{}
	cmd := &cobra.Command{
		Use:   "web",
		Short: "Start the local AgentFlow web server",
		Long: "Launch the local AgentFlow web server bound to 127.0.0.1 with a one-shot session token. " +
			"Configuration is merged from defaults, ~/.agentflow/settings.toml, the AGENTFLOW_WEB_* environment variables, and CLI flags in that order of precedence.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runWebCommand(cmd, flags)
		},
	}
	cmd.Flags().StringVar(&flags.host, "host", settings.DefaultHost, "loopback host to bind")
	cmd.Flags().IntVar(&flags.port, "port", settings.DefaultPort, "port to bind (0 chooses an available port)")
	cmd.Flags().BoolVar(&flags.noOpen, "no-open", false, "do not open the browser automatically")
	cmd.Flags().StringVar(&flags.devAssets, "dev-assets", "", "serve frontend assets from this directory instead of the embedded copy")
	cmd.Flags().StringVar(&flags.daemon, "daemon", string(settings.DaemonRequirementAuto), "agentflowd requirement: auto, required, or off")
	cmd.Flags().StringVar(&flags.root, "root", "", "override the AgentFlow root directory (default ~/.agentflow)")
	cmd.Flags().StringVar(&flags.token, "token", "", "override the generated session token (advanced)")

	cmd.PreRunE = func(cmd *cobra.Command, _ []string) error {
		flags.hostSet = cmd.Flags().Changed("host")
		flags.portSet = cmd.Flags().Changed("port")
		flags.devAssetsSet = cmd.Flags().Changed("dev-assets")
		flags.daemonSet = cmd.Flags().Changed("daemon")
		flags.rootSet = cmd.Flags().Changed("root")
		flags.tokenSet = cmd.Flags().Changed("token")
		return nil
	}

	return cmd
}

func runWebCommand(cmd *cobra.Command, flags *webFlags) error {
	overrides := flags.overrides()
	cfg, err := settings.Load("", settings.NewOSEnv(), overrides)
	if err != nil {
		return err
	}
	command := &web.Command{
		Settings: cfg,
		Logger:   slog.New(slog.NewTextHandler(os.Stderr, nil)),
		Stdout:   cmd.OutOrStdout(),
		OpenURL:  openBrowserURL,
	}
	return command.Run(cmd.Context())
}

func (f *webFlags) overrides() settings.Overrides {
	overrides := settings.Overrides{}
	if f.hostSet {
		host := f.host
		overrides.Host = &host
	}
	if f.portSet {
		port := f.port
		overrides.Port = &port
	}
	if f.noOpen {
		val := true
		overrides.NoOpen = &val
	}
	if f.devAssetsSet {
		dev := f.devAssets
		overrides.DevAssets = &dev
	}
	if f.daemonSet {
		daemon := settings.DaemonRequirement(f.daemon)
		overrides.Daemon = &daemon
	}
	if f.rootSet {
		root := f.root
		overrides.Root = &root
	}
	if f.tokenSet {
		token := f.token
		overrides.Token = &token
	}
	return overrides
}

func openBrowserURL(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "linux":
		cmd = exec.Command("xdg-open", url)
	case "windows":
		cmd = exec.Command("cmd", "/C", "start", "", url)
	default:
		return fmt.Errorf("automatic browser open is not supported on %s", runtime.GOOS)
	}
	return cmd.Start()
}
