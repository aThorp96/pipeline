/*
 Copyright 2025 The Tekton Authors

 Licensed under the Apache License, Version 2.0 (the "License");
 you may not use this file except in compliance with the License.
 You may obtain a copy of the License at

     http://www.apache.org/licenses/LICENSE-2.0

 Unless required by applicable law or agreed to in writing, software
 distributed under the License is distributed on an "AS IS" BASIS,
 WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 See the License for the specific language governing permissions and
 limitations under the License.

*/

package git

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func fileExists(t *testing.T, path string) bool {
	t.Helper()
	_, err := os.Stat(path)
	return !errors.Is(err, os.ErrNotExist)
}

func TestClone(t *testing.T) {
	type testCase struct {
		url       string
		username  string
		password  string
		expectErr string
	}
	// TODO: Skip test if running tests offline?
	testCases := map[string]testCase{
		"normal usage":           {url: "https://github.com/tektoncd/pipeline"},
		"normal usage with .git": {url: "https://github.com/tektoncd/pipeline.git"},
		"private repository":     {url: "https://github.com/tektoncd/not-a-repository.git", expectErr: "clone error: authentication required"},
		// Note: cloning a private repo with authentication is done in the E2E tests
		"with crendentials": {url: "https://github.com/tektoncd/not-a-repository.git", expectErr: "clone error: authentication required", username: "fake", password: "fake"},
	}

	for name, test := range testCases {
		t.Run(name, func(t *testing.T) {
			repo, cleanup, err := clone(context.Background(), test.url, test.username, test.password)
			defer cleanup()
			if test.expectErr != "" {
				if err.Error() != test.expectErr {
					t.Fatalf("Expected error %q but got %q", test.expectErr, err)
				}
			} else {
				if err != nil {
					t.Fatalf("Error cloning repository %q: %v", test.url, err)
				}

				fInfo, err := os.Stat(repo.directory)
				if err != nil {
					t.Fatalf("Could not validate clone directory: %v", err)
				}
				if !fInfo.IsDir() {
					t.Fatal("Expected repo temp directory to be directory")
				}

				if !fileExists(t, filepath.Join(repo.directory, ".git")) {
					t.Fatalf("Clone repository %q does not contain '.git' directory", repo.directory)
				}
			}
		})
	}
}

func TestCheckout(t *testing.T) {
	repoPath, revisions := createTestRepo(
		t,
		[]commitForRepo{
			{
				Filename: "README.md",
				Content:  "some content",
				Branch:   "non-main",
				Tag:      "1.0.0",
			},
			{
				Filename: "otherfile.yaml",
				Content:  "some data",
				Branch:   "to-be-deleted",
			},
		},
	)
	gitCmd := getGitCmd(t, repoPath)
	if err := gitCmd("checkout", "main").Run(); err != nil {
		t.Fatalf("cloud not checkout main branch after repo initialization: %v", err)
	}
	if err := gitCmd("branch", "-D", "to-be-deleted").Run(); err != nil {
		t.Fatalf("coun't delete branch to orphan commit: %v", err)
	}

	ctx := context.Background()

	type testCase struct {
		revision         string
		expectedRevision string
		expectErr        string
	}
	testCases := map[string]testCase{
		"revision is branch":          {revision: "non-main", expectedRevision: revisions[0]},
		"revision is tag":             {revision: "1.0.0", expectedRevision: revisions[0]},
		"revision is sha":             {revision: revisions[0], expectedRevision: revisions[0]},
		"revision is unreachable sha": {revision: revisions[1], expectedRevision: revisions[1]},
		"non-existent revision":       {revision: "fake-revision", expectErr: "git fetch error: fatal: couldn't find remote ref fake-revision: exit status 128"},
	}

	for name, test := range testCases {
		t.Run(name, func(t *testing.T) {
			repo, cleanup, err := clone(ctx, repoPath, "", "")
			defer cleanup()

			if err != nil {
				t.Fatalf("Error cloning repository %v", err)
			}

			err = repo.checkout(ctx, test.revision)
			if test.expectErr != "" {
				if err == nil {
					t.Fatal("Expected error checking out revision but got none")
				} else if err.Error() != test.expectErr {
					t.Fatalf("Expected error %q but got %q", test.expectErr, err)
				}
				return
			} else if err != nil {
				t.Fatalf("Error checking out revision: %v", err)
			}

			revision, err := repo.currentRevision(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if revision != test.expectedRevision {
				t.Fatalf("Expected revision to be %q but got %q", test.expectedRevision, revision)
			}
		})
	}
}
