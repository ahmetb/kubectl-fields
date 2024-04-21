// Copyright 2024 Ahmet Alp Balkan
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

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
				Clock: testingclock.NewFakePassiveClock(time.Date(2024, 4, 10, 1, 34, 50, 0, time.UTC)),
			},
			outFile: "1_deployment_inline.out",
		},
		{
			name:    "Deployment above",
			inFile:  "1_deployment.yaml",
			errFunc: require.NoError,
			opts: annotationOptions{
				Clock:    testingclock.NewFakePassiveClock(time.Date(2024, 4, 10, 17, 40, 04, 0, time.UTC)),
				Position: Above,
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
