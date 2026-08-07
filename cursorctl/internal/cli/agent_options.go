package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

type agentOptionFlags struct {
	sharedSendFlags
	name                string
	agentID             string
	cwd                 string
	dirs                []string
	settingSources      []string
	sandbox             bool
	autoReview          bool
	repos               []string
	prURL               string
	envType             string
	envName             string
	autoCreatePR        bool
	skipReviewerRequest bool
	workOnCurrentBranch bool
	envVars             []string
	metadata            []string
	openAsGitHubApp     bool
	agentsConfig        string
	tools               []string
	disallowedTools     []string
}

var localAgentFlagNames = []string{
	"cwd", "dir", "setting-source", "sandbox", "auto-review",
}

var cloudAgentFlagNames = []string{
	"repo", "pr-url", "env-type", "env-name", "auto-create-pr",
	"skip-reviewer-request", "work-on-current-branch", "env-var", "metadata",
	"open-as-github-app",
}

func addAgentOptionFlags(command *cobra.Command, flags *agentOptionFlags) {
	set := command.Flags()
	set.StringVar(&flags.model, "model", "", "Model identifier")
	set.StringVar(&flags.name, "name", "", "Agent name")
	set.StringVar(&flags.agentID, "agent-id", "", "Explicit agent ID in AgentOptions")
	set.StringVar(&flags.mode, "mode", "", "Conversation mode: agent or plan")
	set.StringVar(&flags.cwd, "cwd", "", "Primary local working directory")
	set.StringArrayVar(&flags.dirs, "dir", nil, "Additional local workspace directory (repeatable)")
	set.StringArrayVar(&flags.settingSources, "setting-source", nil, "Local setting source (repeatable)")
	set.BoolVar(&flags.sandbox, "sandbox", false, "Enable the local sandbox")
	set.BoolVar(&flags.autoReview, "auto-review", false, "Enable local automatic tool review")
	set.StringArrayVar(&flags.repos, "repo", nil, "Cloud repository URL[@ref] (repeatable)")
	set.StringVar(&flags.prURL, "pr-url", "", "Pull request URL for the single cloud repository")
	set.StringVar(&flags.envType, "env-type", "", "Cloud environment type: cloud, pool, or machine")
	set.StringVar(&flags.envName, "env-name", "", "Cloud environment name")
	set.BoolVar(&flags.autoCreatePR, "auto-create-pr", false, "Create a pull request automatically")
	set.BoolVar(&flags.skipReviewerRequest, "skip-reviewer-request", false, "Skip automatic reviewer requests")
	set.BoolVar(&flags.workOnCurrentBranch, "work-on-current-branch", false, "Work on the repository's current branch")
	set.StringArrayVar(&flags.envVars, "env-var", nil, "Cloud agent environment KEY=VAL (repeatable)")
	set.StringArrayVar(&flags.metadata, "metadata", nil, "Cloud agent metadata KEY=VAL (repeatable)")
	set.BoolVar(&flags.openAsGitHubApp, "open-as-github-app", false, "Open pull requests as the Cursor GitHub App")
	set.StringVar(&flags.mcpConfig, "mcp-config", "", "MCP server map as JSON, @file, or -")
	set.StringVar(&flags.agentsConfig, "agents-config", "", "Sub-agent definition map as JSON, @file, or -")
	set.StringArrayVar(&flags.tools, "tool", nil, "Allowed built-in tool (repeatable)")
	set.StringArrayVar(&flags.disallowedTools, "disallowed-tool", nil, "Disallowed built-in tool (repeatable)")
}

func buildAgentOptions(
	app *App,
	command *cobra.Command,
	flags *agentOptionFlags,
	apiKey string,
) (map[string]any, error) {
	local := anyFlagChanged(command, localAgentFlagNames)
	cloud := anyFlagChanged(command, cloudAgentFlagNames)
	if local && cloud {
		return nil, fmt.Errorf("cannot combine local and cloud agent flags")
	}

	options := map[string]any{"apiKey": apiKey}
	if command.Flags().Changed("model") {
		options["model"] = map[string]any{"id": flags.model}
	}
	if command.Flags().Changed("name") {
		options["name"] = flags.name
	}
	if command.Flags().Changed("agent-id") {
		options["agentId"] = flags.agentID
	}
	if command.Flags().Changed("mode") {
		// proto/sdk/v1/sdk_messages.proto defines AGENT_MODE_OPTION_* literals.
		mode, err := prefixedEnum(flags.mode, "AGENT_MODE_OPTION_", "AGENT", "PLAN")
		if err != nil {
			return nil, err
		}
		options["mode"] = mode
	}
	if command.Flags().Changed("mcp-config") {
		config, err := app.ReadJSONPayload(flags.mcpConfig)
		if err != nil {
			return nil, fmt.Errorf("read --mcp-config: %w", err)
		}
		options["mcpServers"] = config
	}
	if command.Flags().Changed("agents-config") {
		config, err := app.ReadJSONPayload(flags.agentsConfig)
		if err != nil {
			return nil, fmt.Errorf("read --agents-config: %w", err)
		}
		options["agents"] = config
	}
	if command.Flags().Changed("tool") {
		options["tools"] = map[string]any{"names": flags.tools}
	}
	if command.Flags().Changed("disallowed-tool") {
		options["disallowedTools"] = flags.disallowedTools
	}

	if cloud {
		cloudOptions, err := buildCloudAgentOptions(command, flags)
		if err != nil {
			return nil, err
		}
		options["cloud"] = cloudOptions
		return options, nil
	}

	localOptions, err := buildLocalAgentOptions(app, command, flags)
	if err != nil {
		return nil, err
	}
	options["local"] = localOptions
	return options, nil
}

func buildLocalAgentOptions(
	app *App,
	command *cobra.Command,
	flags *agentOptionFlags,
) (map[string]any, error) {
	cwd := flags.cwd
	if !command.Flags().Changed("cwd") {
		cwd = app.Workspace
	}
	local := map[string]any{"cwd": []string{cwd}}
	if command.Flags().Changed("dir") {
		local["dirs"] = flags.dirs
	}
	if command.Flags().Changed("setting-source") {
		sources := make([]string, len(flags.settingSources))
		for index, source := range flags.settingSources {
			value, err := prefixedEnum(
				source,
				"SETTING_SOURCE_",
				"PROJECT",
				"USER",
				"TEAM",
				"MDM",
				"PLUGINS",
				"ALL",
			)
			if err != nil {
				return nil, fmt.Errorf("invalid --setting-source: %w", err)
			}
			sources[index] = value
		}
		local["settingSources"] = sources
	}
	if command.Flags().Changed("sandbox") {
		local["sandboxOptions"] = map[string]any{"enabled": flags.sandbox}
	}
	if command.Flags().Changed("auto-review") {
		local["autoReview"] = flags.autoReview
	}
	return local, nil
}

func buildCloudAgentOptions(
	command *cobra.Command,
	flags *agentOptionFlags,
) (map[string]any, error) {
	cloud := map[string]any{}
	if command.Flags().Changed("repo") {
		repos := make([]map[string]any, len(flags.repos))
		for index, value := range flags.repos {
			repo, err := parseRepo(value)
			if err != nil {
				return nil, err
			}
			repos[index] = repo
		}
		if command.Flags().Changed("pr-url") {
			if len(repos) != 1 {
				return nil, fmt.Errorf("--pr-url requires exactly one --repo")
			}
			repos[0]["prUrl"] = flags.prURL
		}
		cloud["repos"] = repos
	} else if command.Flags().Changed("pr-url") {
		return nil, fmt.Errorf("--pr-url requires exactly one --repo")
	}
	if command.Flags().Changed("env-type") || command.Flags().Changed("env-name") {
		env := map[string]any{}
		if command.Flags().Changed("env-type") {
			// proto/sdk/v1/sdk_messages.proto defines CLOUD_ENVIRONMENT_TYPE_* literals.
			value, err := prefixedEnum(
				flags.envType,
				"CLOUD_ENVIRONMENT_TYPE_",
				"CLOUD",
				"POOL",
				"MACHINE",
			)
			if err != nil {
				return nil, err
			}
			env["type"] = value
		}
		if command.Flags().Changed("env-name") {
			env["name"] = flags.envName
		}
		cloud["env"] = env
	}
	setOptionalBool(command, cloud, "auto-create-pr", "autoCreatePr", flags.autoCreatePR)
	setOptionalBool(command, cloud, "skip-reviewer-request", "skipReviewerRequest", flags.skipReviewerRequest)
	setOptionalBool(command, cloud, "work-on-current-branch", "workOnCurrentBranch", flags.workOnCurrentBranch)
	setOptionalBool(command, cloud, "open-as-github-app", "openAsCursorGithubApp", flags.openAsGitHubApp)
	if command.Flags().Changed("env-var") {
		values, err := parseKeyValues(flags.envVars)
		if err != nil {
			return nil, fmt.Errorf("parse --env-var: %w", err)
		}
		cloud["envVars"] = values
	}
	if command.Flags().Changed("metadata") {
		values, err := parseKeyValues(flags.metadata)
		if err != nil {
			return nil, fmt.Errorf("parse --metadata: %w", err)
		}
		cloud["metadata"] = values
	}
	return cloud, nil
}

func anyFlagChanged(command *cobra.Command, names []string) bool {
	for _, name := range names {
		if command.Flags().Changed(name) {
			return true
		}
	}
	return false
}

func setOptionalBool(
	command *cobra.Command,
	target map[string]any,
	flagName string,
	fieldName string,
	value bool,
) {
	if command.Flags().Changed(flagName) {
		target[fieldName] = value
	}
}
