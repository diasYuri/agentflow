package cli

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/diasYuri/agentflow/internal/app"
	"github.com/diasYuri/agentflow/internal/slack"
	"github.com/diasYuri/agentflow/internal/web/settings"
)

type slackFlags struct {
	appToken     string
	botToken     string
	project      string
	root         string
	daemon       string
	daemonSocket string
	logToStdout  bool

	appTokenSet     bool
	botTokenSet     bool
	projectSet      bool
	rootSet         bool
	daemonSet       bool
	daemonSocketSet bool
}

func newSlackCommand() *cobra.Command {
	flags := &slackFlags{}
	cmd := &cobra.Command{
		Use:   "slack",
		Short: "Start the local AgentFlow Slack bot",
		Long:  "Connect AgentFlow to Slack via Socket Mode. The bot shares the same channel-neutral agent flow as the web adapter, but listens on Slack events instead of HTTP.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSlackCommand(cmd, flags)
		},
	}
	cmd.Flags().StringVar(&flags.appToken, "app-token", "", "Slack app-level token (xapp-...)")
	cmd.Flags().StringVar(&flags.botToken, "bot-token", "", "Slack bot token (xoxb-...)")
	cmd.Flags().StringVar(&flags.project, "project", "", "project name used for Slack sessions")
	cmd.Flags().StringVar(&flags.root, "root", "", "override the AgentFlow root directory (default ~/.agentflow)")
	cmd.Flags().StringVar(&flags.daemon, "daemon", string(settings.DaemonRequirementAuto), "agentflowd requirement: auto, required, or off")
	cmd.Flags().StringVar(&flags.daemonSocket, "daemon-socket", "", "override the agentflowd unix socket")
	cmd.Flags().BoolVarP(&flags.logToStdout, "log", "l", false, "send backend logs to stdout")

	cmd.PreRunE = func(cmd *cobra.Command, _ []string) error {
		flags.appTokenSet = cmd.Flags().Changed("app-token")
		flags.botTokenSet = cmd.Flags().Changed("bot-token")
		flags.projectSet = cmd.Flags().Changed("project")
		flags.rootSet = cmd.Flags().Changed("root")
		flags.daemonSet = cmd.Flags().Changed("daemon")
		flags.daemonSocketSet = cmd.Flags().Changed("daemon-socket")
		return nil
	}

	return cmd
}

func runSlackCommand(cmd *cobra.Command, flags *slackFlags) error {
	overrides := flags.overrides()
	cfg, err := settings.Load("", settings.NewOSEnv(), overrides)
	if err != nil {
		return err
	}
	if flags.daemonSocketSet {
		cfg.Paths.DaemonSocket = flags.daemonSocket
	}
	appToken := cfg.Slack.AppToken
	if flags.appTokenSet {
		appToken = flags.appToken
	}
	botToken := cfg.Slack.BotToken
	if flags.botTokenSet {
		botToken = flags.botToken
	}
	project := cfg.Slack.Project
	if flags.projectSet {
		project = flags.project
	}
	if strings.TrimSpace(appToken) == "" {
		return fmt.Errorf("slack app token is required; set --app-token or AGENTFLOW_SLACK_APP_TOKEN")
	}
	if strings.TrimSpace(botToken) == "" {
		return fmt.Errorf("slack bot token is required; set --bot-token or AGENTFLOW_SLACK_BOT_TOKEN")
	}
	if strings.TrimSpace(project) == "" {
		return fmt.Errorf("slack project is required; set --project or AGENTFLOW_SLACK_PROJECT")
	}
	var logOutput io.Writer = os.Stderr
	if flags.logToStdout {
		logOutput = cmd.OutOrStdout()
	}
	command := &slack.Command{
		Settings:    cfg,
		AppToken:    appToken,
		BotToken:    botToken,
		ProjectName: project,
		Logger:      slog.New(slog.NewTextHandler(logOutput, nil)),
		Stdout:      cmd.OutOrStdout(),
		Daemon:      slack.NewDefaultDaemonChecker(cfg.Paths.DaemonSocket),
		Projects:    app.NewProjectRegistry(nil),
	}
	return command.Run(cmd.Context())
}

func (f *slackFlags) overrides() settings.Overrides {
	overrides := settings.Overrides{}
	if f.rootSet {
		root := f.root
		overrides.Root = &root
	}
	if f.daemonSet {
		daemon := settings.DaemonRequirement(f.daemon)
		overrides.Daemon = &daemon
	}
	return overrides
}
