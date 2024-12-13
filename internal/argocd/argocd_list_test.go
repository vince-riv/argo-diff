package argocd

import (
	"context"
	"fmt"
	"testing"
)

// argocd_testdata/output-argocd-list-applications-brief.json

const outputListApplications = "output-argocd-list-applications-brief.json"

func getMockedArgoCdCli(outputFile string) (func(context.Context, []string) ([]byte, error), error) {
	data, filePath, err := readFileToByteArray(outputFile)
	if err != nil {
		return nil, fmt.Errorf("error reading file '%s': %w", filePath, err)
	}
	mockedFunction := func(ctx context.Context, args []string) ([]byte, error) {
		return data, nil
	}
	return mockedFunction, nil
}

func TestArgoCdCliListApplications(t *testing.T) {
	var err error
	originalExecArgoCdCli := execArgoCdCli
	execArgoCdCli, err = getMockedArgoCdCli(outputListApplications)
	defer func() { execArgoCdCli = originalExecArgoCdCli }()
	if err != nil {
		t.Errorf("Failed reading %s: %v", outputListApplications, err)
	}
	ctx := context.Background()
	appApps, err := listApplications(ctx)
	if err != nil {
		t.Errorf("listApplications() failed: %v", err)
	}
	if len(appApps.Items) != 2 {
		t.Errorf("listApplications() returned %d items, 2 expected", len(appApps.Items))
	}
}
