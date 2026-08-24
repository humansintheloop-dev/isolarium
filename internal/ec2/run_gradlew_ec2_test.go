//go:build ec2

// The file name is load-bearing. It sorts after repo_ec2_test.go, because the
// build writes testdata/spring-boot-app/build and .gradle into the clone and
// repo_ec2_test.go asserts the clone is clean; and ahead of tmux_ec2_test.go,
// whose tests deliberately leave a process running in the shared tmux session
// this run needs free.
package ec2_test

import (
	"strings"
	"testing"
	"time"
)

const (
	springBootAppDir     = "testdata/spring-boot-app"
	gradlewPath          = springBootAppDir + "/gradlew"
	gradleBuildSucceeded = "BUILD SUCCESSFUL"

	// gradlewBuildTimeout allows for the first run's download of the Gradle
	// distribution and of every dependency, on top of the build itself.
	gradlewBuildTimeout = 20 * time.Minute
)

// TestEC2Run_GradlewBuildsTheSpringBootApp proves the Java toolchain the
// instance carries is enough for a real workload: the Spring Boot app that
// arrives with the clone of this repository builds under its own gradlew.
func TestEC2Run_GradlewBuildsTheSpringBootApp(t *testing.T) {
	environment := sharedInstance(t)
	environment.clearWhatAnEarlierTmuxTestLeftBehind()
	environment.assertTheCloneCarriesTheSpringBootApp()

	started := time.Now()
	build := environment.startIsolariumRun(gradlewBuildCommand()...)
	exitCode := build.waitUntilItEnds(gradlewBuildTimeout)
	environment.t.Logf("TIMING: gradlew clean build took %s", time.Since(started).Round(time.Second))

	if exitCode != 0 {
		t.Errorf("%s exited %d, want 0\n%s", build.label, exitCode, visibleText(build.output()))
	}
	if !strings.Contains(visibleText(build.output()), gradleBuildSucceeded) {
		t.Errorf("%s did not print %q:\n%s", build.label, gradleBuildSucceeded, visibleText(build.output()))
	}
}

// gradlewBuildCommand sources SDKMAN first because the Java that gradlew needs
// on its PATH is the one SDKMAN installed, and a non-interactive shell does not
// load it on its own.
func gradlewBuildCommand() []string {
	return []string{"bash", "-c", "source ~/.sdkman/bin/sdkman-init.sh && cd " + springBootAppDir + " && ./gradlew clean build"}
}

// assertTheCloneCarriesTheSpringBootApp fails with a clear message when the
// checkout on the instance is missing the app, rather than leaving that to a
// gradle error deep in the build output.
func (e *ec2Environment) assertTheCloneCarriesTheSpringBootApp() {
	e.t.Helper()

	if exitCode, _ := e.askInstanceInRepo("test", "-f", gradlewPath); exitCode != 0 {
		e.t.Fatalf("%s is missing from the clone on the instance; the checkout did not bring the Spring Boot app", gradlewPath)
	}
}
