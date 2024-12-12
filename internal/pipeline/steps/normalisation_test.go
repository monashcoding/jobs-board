package steps

import (
	pipelinetesting "github.com/monashcoding/mploy-golang-core/internal/pipeline/testing"
	"testing"
)

func TestNormalizationStep_Process(t *testing.T) {
	step := NewNormalizationStep()
	suite := pipelinetesting.NewPipelineStepTestSuite(t, step, "testdata/normalization")

	testCases := []pipelinetesting.TestCase{
		{
			Name:       "clean_data",
			InputFile:  "clean/input.json",
			OutputFile: "clean/output.json",
		},
	}

	for _, tc := range testCases {
		suite.RunTestCase(tc)
	}
}
