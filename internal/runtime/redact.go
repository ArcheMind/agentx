package runtime

import (
	"regexp"
	"strings"
)

var credentialEnvName = regexp.MustCompile(`(?i)(KEY|TOKEN|SECRET|PASSWORD|PASSWD|CREDENTIAL|AUTH|COOKIE|AWS_PROFILE)`)

var secretPatterns = []*regexp.Regexp{
	regexp.MustCompile(`sk-[A-Za-z0-9_\-]{16,}`),
	regexp.MustCompile(`eyJ[A-Za-z0-9_\-]{8,}\.[A-Za-z0-9_\-]{8,}\.[A-Za-z0-9_\-]{8,}`),
}

var labeledSecretPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)(bearer\s+)[A-Za-z0-9._~+/\-]{16,}=*`),
	regexp.MustCompile(`(?i)("(?:access_token|refresh_token|id_token|api_key|apiKey|token|key|secret|password|credential)"\s*:\s*")[^"]+`),
}

// CredentialEnvNames lists inherited variables that may carry credentials so external runtimes
// authenticate only through their explicitly configured source.
func CredentialEnvNames(environ []string) []string {
	names := []string{}
	for _, item := range environ {
		key, _, ok := strings.Cut(item, "=")
		if ok && credentialEnvName.MatchString(key) {
			names = append(names, key)
		}
	}
	return names
}

func RedactSecrets(text string) string {
	for _, pattern := range secretPatterns {
		text = pattern.ReplaceAllString(text, "<redacted>")
	}
	for _, pattern := range labeledSecretPatterns {
		text = pattern.ReplaceAllString(text, "${1}<redacted>")
	}
	return text
}

func redactPlan(plan CommandPlan) CommandPlan {
	redacted := plan
	redacted.Args = make([]string, len(plan.Args))
	for i, arg := range plan.Args {
		redacted.Args[i] = RedactSecrets(arg)
	}
	if plan.Env != nil {
		redacted.Env = map[string]string{}
		for key, value := range plan.Env {
			if credentialEnvName.MatchString(key) {
				value = "<redacted>"
			}
			redacted.Env[key] = RedactSecrets(value)
		}
	}
	return redacted
}
