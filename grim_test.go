package grim

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Copyright 2015 MediaMath <http://www.mediamath.com>.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

func TestTruncatedGrimServerID(t *testing.T) {
	var buf bytes.Buffer
	logger := log.New(&buf, "", log.Lshortfile)

	tempDir, err := ioutil.TempDir("", "TestTruncatedGrimServerID")
	if err != nil {
		t.Errorf("|%v|", err)
	}
	defer os.RemoveAll(tempDir)

	// GrimQueueName is set to be 20 chars long, which should get truncated
	configJS := `{"GrimQueueName":"12345678901234567890","AWSRegion":"empty","AWSKey":"empty","AWSSecret":"empty"}`
	ioutil.WriteFile(filepath.Join(tempDir, "config.json"), []byte(configJS), 0644)

	g := &Instance{
		configRoot: &tempDir,
		queue:      nil,
	}

	g.PrepareGrimQueue(logger)
	message := fmt.Sprintf("%v", &buf)

	if !strings.Contains(message, buildTruncatedMessage("GrimQueueName")) {
		t.Errorf("Failed to log truncation of grimServerID")
	}
}

func TestTimeOutConfig(t *testing.T) {
	if testing.Short() {
		t.Skipf("Skipping prepare test in short mode.")
	}

	tempDir, err := ioutil.TempDir("", "TestTimeOut")
	if err != nil {
		t.Errorf("|%v|", err)
	}
	defer os.RemoveAll(tempDir)

	configJS := `{"Timeout":4,"AWSRegion":"empty","AWSKey":"empty","AWSSecret":"empty"}`
	ioutil.WriteFile(filepath.Join(tempDir, "config.json"), []byte(configJS), 0644)

	config, err := getEffectiveGlobalConfig(tempDir)
	if err != nil {
		t.Errorf("|%v|", err)
	}
	config.resultRoot = tempDir
	if config.timeout == int(defaultTimeout.Seconds()) {
		t.Errorf("Failed to use non default timeout time")
	}

	err = doWaitAction(config, testOwner, testRepo, 2)
	if err != nil {
		t.Errorf("Failed to not timeout")
	}
}

func doWaitAction(config *effectiveConfig, owner, repo string, wait int) error {
	return onHookBuild("not-used", config, hookEvent{Owner: owner, Repo: repo}, nil, func(r string, resultPath string, c *effectiveConfig, h hookEvent, s string) (*executeResult, string, error) {
		time.Sleep(time.Duration(wait) * time.Second)
		return &executeResult{}, "", nil
	})
}

func TestBuildRef(t *testing.T) {
	if testing.Short() {
		t.Skipf("Skipping prepare test in short mode.")
	}

	owner := "MediaMath"
	repo := "grim"
	ref := "test" //special grim branch
	clonePath := "go/src/github.com/MediaMath/grim"

	temp, _ := ioutil.TempDir("", "TestBuildRef")

	configRoot := filepath.Join(temp, "config")
	os.MkdirAll(filepath.Join(configRoot, owner, repo), 0700)

	grimConfigTemplate := `{
		"ResultRoot": "%v",
		"WorkspaceRoot": "%v",
		"AWSRegion": "bogus",
		"AWSKey": "bogus",
		"AWSSecret": "bogus"
	}`
	grimJs := fmt.Sprintf(grimConfigTemplate, filepath.Join(temp, "results"), filepath.Join(temp, "ws"))

	ioutil.WriteFile(filepath.Join(configRoot, "config.json"), []byte(grimJs), 0644)

	localConfigTemplate := `{
		"PathToCloneIn": "%v"
	}`
	localJs := fmt.Sprintf(localConfigTemplate, clonePath)

	ioutil.WriteFile(filepath.Join(configRoot, owner, repo, "config.json"), []byte(localJs), 0644)
	var g Instance
	g.SetConfigRoot(configRoot)

	logfile, err := os.OpenFile(filepath.Join(temp, "log.txt"), os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
	if err != nil {
		t.Fatalf("error opening file: %v", err)
	}

	logger := log.New(logfile, "", log.Ldate|log.Ltime)
	buildErr := g.BuildRef(owner, repo, ref, logger)
	logfile.Close()

	if buildErr != nil {
		t.Errorf("%v: %v", temp, buildErr)
	}

	if !t.Failed() {
		os.RemoveAll(temp)
	}
}

var testOwner = "MediaMath"
var testRepo = "grim"

func TestOnActionFailure(t *testing.T) {
	tempDir, _ := ioutil.TempDir("", "results-dir-failure")
	defer os.RemoveAll(tempDir)

	doNothingAction(tempDir, testOwner, testRepo, 123, nil)

	if _, err := resultsDirectoryExists(tempDir, testOwner, testRepo); err != nil {
		t.Errorf("|%v|", err)
	}

}

func TestOnActionError(t *testing.T) {
	tempDir, _ := ioutil.TempDir("", "results-dir-error")
	defer os.RemoveAll(tempDir)

	doNothingAction(tempDir, testOwner, testRepo, 0, fmt.Errorf("Bad Bad thing happened"))

	if _, err := resultsDirectoryExists(tempDir, testOwner, testRepo); err != nil {
		t.Errorf("|%v|", err)
	}
}

func TestResultsDirectoryCreatedInOnHook(t *testing.T) {
	tempDir, _ := ioutil.TempDir("", "results-dir-success")
	defer os.RemoveAll(tempDir)

	doNothingAction(tempDir, testOwner, testRepo, 0, nil)

	if _, err := resultsDirectoryExists(tempDir, testOwner, testRepo); err != nil {
		t.Errorf("|%v|", err)
	}
}

func TestHookGetsLogged(t *testing.T) {
	tempDir, _ := ioutil.TempDir("", "results-dir-success")
	defer os.RemoveAll(tempDir)

	hook := hookEvent{Owner: testOwner, Repo: testRepo, StatusRef: "fooooooooooooooooooo"}

	err := onHookBuild("not-used", &effectiveConfig{resultRoot: tempDir}, hook, nil, func(r string, resultPath string, c *effectiveConfig, h hookEvent, s string) (*executeResult, string, error) {
		return &executeResult{ExitCode: 0}, "", nil
	})

	if err != nil {
		t.Fatalf("%v", err)
	}

	results, _ := resultsDirectoryExists(tempDir, testOwner, testRepo)
	hookFile := filepath.Join(results, "hook.json")

	if _, err := os.Stat(hookFile); os.IsNotExist(err) {
		t.Errorf("%s was not created.", hookFile)
	}

	jsonHookFile, readerr := ioutil.ReadFile(hookFile)
	if readerr != nil {
		t.Errorf("Error reading file %v", readerr)
	}

	var parsed hookEvent
	parseErr := json.Unmarshal(jsonHookFile, &parsed)
	if parseErr != nil {
		t.Errorf("Error parsing: %v", parseErr)
	}

	if hook.Owner != parsed.Owner || hook.Repo != parsed.Repo || hook.StatusRef != parsed.StatusRef {
		t.Errorf("Did not match:\n%v\n%v", hook, parsed)
	}

}

func TestShouldSkip(t *testing.T) {
	var skipTests = []struct {
		in   *hookEvent
		retn bool // True for nil, False for not nil
	}{
		{&hookEvent{Deleted: true}, false},
		{&hookEvent{Deleted: true, EventName: "push"}, false},
		{&hookEvent{Deleted: true, EventName: "pull_request"}, false},
		{&hookEvent{Deleted: true, EventName: "pull_request", Action: "reopened"}, false},
		{&hookEvent{EventName: "push"}, true},
		{&hookEvent{EventName: "push", Action: "opened"}, true},
		{&hookEvent{EventName: "push", Action: "doesn't matter"}, true},
		{&hookEvent{EventName: "pull_request", Action: "opened"}, true},
		{&hookEvent{EventName: "pull_request", Action: "reopened"}, true},
		{&hookEvent{EventName: "pull_request", Action: "synchronize"}, true},
		{&hookEvent{EventName: "pull_request", Action: "matters"}, false},
		{&hookEvent{EventName: "issue", Action: "opened"}, false},
	}
	for _, sT := range skipTests {
		message := shouldSkip(sT.in)
		if XOR(message == nil, sT.retn) {
			t.Errorf("Failed test for hook with params<Deleted:%t,EventName:%v,Action:%v> with message:%d", sT.in.Deleted, sT.in.EventName, sT.in.Action, message)
		}
	}
}

func XOR(a, b bool) bool {
	return a != b
}

func doNothingAction(tempDir, owner, repo string, exitCode int, returnedErr error) error {
	return onHookBuild("not-used", &effectiveConfig{resultRoot: tempDir}, hookEvent{Owner: owner, Repo: repo}, nil, func(r string, resultPath string, c *effectiveConfig, h hookEvent, s string) (*executeResult, string, error) {
		return &executeResult{ExitCode: exitCode}, "", returnedErr
	})
}

func resultsDirectoryExists(tempDir, owner, repo string) (string, error) {
	files, err := ioutil.ReadDir(tempDir)
	if err != nil {
		return "", err
	}

	var fileNames []string
	for _, stat := range files {
		fileNames = append(fileNames, stat.Name())
	}

	repoResults := filepath.Join(tempDir, owner, repo)

	if _, err := os.Stat(repoResults); os.IsNotExist(err) {
		return "", fmt.Errorf("%s was not created: %s", repoResults, fileNames)
	}

	baseFiles, err := ioutil.ReadDir(repoResults)
	if len(baseFiles) != 1 {
		return "", fmt.Errorf("Did not create base name in repo results")
	}

	return filepath.Join(repoResults, baseFiles[0].Name()), nil
}
