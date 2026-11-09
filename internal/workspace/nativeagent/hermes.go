package nativeagent

func prepareHermes(cfg Config, home string, env []string) (invocation, string) {
	var args []string

	if cfg.AgentKind != "hermes" || cfg.HermesModel == "" {
		return invocation{}, "PROVIDER_NOT_CONFIGURED"
	}
	env = append(env, "HERMES_HOME="+home, "HERMES_MODEL="+cfg.HermesModel, "INFERENCE_URL="+cfg.InferenceURL, "WORKBENCH_INFERENCE_GRANT="+cfg.Token, "TERMINAL_ENV=local", "PYTHONPATH=/opt/hermes", "PYTHONUNBUFFERED=1")

	return invocation{executable: "/usr/local/bin/hermes-runner", args: args, env: env, resultFile: ""}, ""
}
