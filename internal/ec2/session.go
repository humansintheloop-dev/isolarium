package ec2

import (
	"encoding/base64"
	"encoding/json"
)

// RemoteClaudeDir is where `claude` keeps its state on the instance.
const RemoteClaudeDir = "~/.claude"

// RemoteCredentialsPath is the credential file `claude` reads on the instance,
// and rewrites there whenever it refreshes its own token.
const RemoteCredentialsPath = RemoteClaudeDir + "/.credentials.json"

// claudeCredentials is one Claude credential blob — the host's copy or the
// instance's own — and knows only what decides which of two should survive.
type claudeCredentials struct {
	blob string
}

// credentialExpiry is the part of a credential blob that says when it expires.
// The value is issued by the auth server and travels inside the blob, so
// comparing two of them compares two values from one clock rather than a
// laptop's against an instance's.
type credentialExpiry struct {
	ClaudeAiOauth struct {
		ExpiresAt *int64 `json:"expiresAt"`
	} `json:"claudeAiOauth"`
}

// expiresAt reports no answer for anything it cannot read an expiry out of: a
// blob that is absent, that is not JSON, or that carries no expiresAt.
func (c claudeCredentials) expiresAt() (int64, bool) {
	var parsed credentialExpiry
	if err := json.Unmarshal([]byte(c.blob), &parsed); err != nil {
		return 0, false
	}
	if parsed.ClaudeAiOauth.ExpiresAt == nil {
		return 0, false
	}
	return *parsed.ClaudeAiOauth.ExpiresAt, true
}

// replaces reports whether this blob should overwrite the one already in place.
// It always does when that one has no readable expiry — absent, unparseable, or
// missing the field — and otherwise only when it expires strictly later. A tie
// leaves the existing copy alone, because a session running against it may have
// refreshed the token itself.
func (c claudeCredentials) replaces(other claudeCredentials) bool {
	otherExpiresAt, otherIsReadable := other.expiresAt()
	if !otherIsReadable {
		return true
	}
	expiresAt, readable := c.expiresAt()
	return readable && expiresAt > otherExpiresAt
}

// encoded is the blob in a form that survives the remote shell, which re-parses
// everything ssh sends it: no quoting, newline, or metacharacter inside a
// base64 payload can alter the command carrying it.
func (c claudeCredentials) encoded() string {
	return base64.StdEncoding.EncodeToString([]byte(c.blob))
}

// ReadInstanceExpiresAt reports when the credentials already on the instance
// expire, and whether there is any answer to be had at all.
func (q InstanceQuery) ReadInstanceExpiresAt() (int64, bool) {
	return q.instanceCredentials().expiresAt()
}

// instanceCredentials reads the instance's own credential file. An instance that
// cannot produce one yields an empty blob, which no comparison can favour.
func (q InstanceQuery) instanceCredentials() claudeCredentials {
	output, exitCode, err := q.capture(readCredentialsCommand())
	if err != nil || exitCode != 0 {
		return claudeCredentials{}
	}
	return claudeCredentials{blob: output}
}

// CopyClaudeCredentials places the host's Claude credentials on the instance
// unless the instance already holds a copy that expires no earlier. Leaving that
// copy alone protects a long-running session there that refreshed its own token
// mid-flight, which an unconditional write would revoke.
func (q InstanceQuery) CopyClaudeCredentials(hostBlob string) error {
	host := claudeCredentials{blob: hostBlob}
	if !host.replaces(q.instanceCredentials()) {
		return nil
	}
	return q.writeCredentials(host)
}

// writeCredentials leaves the instance holding the host's credentials in a file
// only its owner can read.
func (q InstanceQuery) writeCredentials(credentials claudeCredentials) error {
	if err := q.mustRun(createClaudeDirCommand(), "create "+RemoteClaudeDir); err != nil {
		return err
	}
	if err := q.mustRun(writeCredentialsCommand(credentials), "write "+RemoteCredentialsPath); err != nil {
		return err
	}
	return q.mustRun(restrictCredentialsCommand(), "restrict "+RemoteCredentialsPath)
}

// readCredentialsCommand discards the remote error stream, because an instance
// that has no credential file yet is the ordinary first-run case rather than
// something worth reporting to the terminal.
func readCredentialsCommand() RemoteCommand {
	return RemoteCommand{Args: []string{"cat", RemoteCredentialsPath, "2>/dev/null"}}
}

func createClaudeDirCommand() RemoteCommand {
	return RemoteCommand{Args: []string{"mkdir", "-p", RemoteClaudeDir}}
}

func writeCredentialsCommand(credentials claudeCredentials) RemoteCommand {
	script := "echo " + credentials.encoded() + " | base64 -d > " + RemoteCredentialsPath
	return RemoteCommand{Args: []string{"bash", "-c", shellQuote(script)}}
}

func restrictCredentialsCommand() RemoteCommand {
	return RemoteCommand{Args: []string{"chmod", "600", RemoteCredentialsPath}}
}
