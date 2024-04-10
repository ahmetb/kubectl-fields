package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	testingclock "k8s.io/utils/clock/testing"
)

func TestE2E(t *testing.T) {
	cases := []struct {
		name    string
		inFile  string
		opts    annotationOptions
		errFunc require.ErrorAssertionFunc
		outFile string
	}{
		{
			name:    "no managed fields",
			inFile:  "0_no_managedFields.yaml",
			errFunc: require.Error,
		},
		{
			name:    "Deployment inline",
			inFile:  "1_deployment.yaml",
			errFunc: require.NoError,
			opts: annotationOptions{
				clock: testingclock.NewFakePassiveClock(time.Date(2024, 4, 10, 1, 34, 50, 0, time.UTC)),
			},
			outFile: "1_deployment_inline.out",
		},
		{
			name:    "Deployment above",
			inFile:  "1_deployment.yaml",
			errFunc: require.NoError,
			opts: annotationOptions{
				clock:    testingclock.NewFakePassiveClock(time.Date(2024, 4, 10, 17, 40, 04, 0, time.UTC)),
				position: Above,
			},
			outFile: "1_deployment_above.out",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in, err := os.ReadFile(filepath.Join("testdata", tc.inFile))
			require.NoError(t, err)

			var buf bytes.Buffer
			err = run(in, &buf, tc.opts)
			tc.errFunc(t, err)
			if err != nil {
				return
			}
			out, err := os.ReadFile(filepath.Join("testdata", tc.outFile))
			require.NoError(t, err)
			require.Equal(t, string(out), buf.String())
		})
	}

}
