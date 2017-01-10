package os

import (
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"sync"

	"github.com/docker/infrakit/pkg/launch"
)

// LaunchConfig is the rule for how to start up a os process.
type LaunchConfig struct {

	// Cmd is the command. This should be in the PATH.
	Cmd string

	// Args are the argument list for the command
	Args []string
}

const (
	// LogDirEnvVar is the environment variable that may be used to customize the plugin logs location
	LogDirEnvVar = "INFRAKIT_LOG_DIR"
)

// DefaultLogDir is the directory for storing the log files from the plugins
func DefaultLogDir() string {
	if logDir := os.Getenv(LogDirEnvVar); logDir != "" {
		return logDir
	}

	home := os.Getenv("HOME")
	if usr, err := user.Current(); err == nil {
		home = usr.HomeDir
	}
	return filepath.Join(home, ".infrakit/logs")
}

// NewLauncher returns a Launcher that can install and start plugins.  The OS version is simple - it translates
// plugin names as command names and uses os.Exec
func NewLauncher(logDir string) (*Launcher, error) {
	return &Launcher{
		logDir:  logDir,
		plugins: map[plugin]state{},
	}, nil
}

type plugin string
type state struct {
	log  string
	wait <-chan error
}

type Launcher struct {
	logDir  string
	plugins map[plugin]state
	lock    sync.Mutex
}

// Name returns the name of the launcher
func (l *Launcher) Name() string {
	return "os"
}

// Launch implements Launcher.Launch.  Returns a signal channel to block on optionally.
// The channel is closed as soon as an error (or nil for success completion) is written.
// The command is run in the background / asynchronously.  The returned read channel
// stops blocking as soon as the command completes (which uses shell to run the real task in
// background).
func (l *Launcher) Launch(name string, config *launch.Config) (<-chan error, error) {

	launchConfig := &LaunchConfig{}
	if err := config.Unmarshal(launchConfig); err != nil {
		return nil, err
	}

	l.lock.Lock()
	defer l.lock.Unlock()

	key := plugin(name)

	if s, has := l.plugins[key]; has {
		return s.wait, nil
	}

	_, err := exec.LookPath(launchConfig.Cmd)
	if err != nil {
		return nil, err
	}

	wait := make(chan error)
	s := state{
		log:  l.buildLogfile(name),
		wait: wait,
	}

	l.plugins[key] = s
	sh := l.buildCmd(s.log, launchConfig.Cmd, launchConfig.Args...)

	startAsync(name, sh, wait)

	return wait, nil
}