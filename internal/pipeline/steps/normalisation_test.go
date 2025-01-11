package steps

import (
	"testing"

	pipelinetesting "github.com/monashcoding/mploy-golang-core/internal/pipeline/testing"
)

func TestNormalizationStep_Process(t *testing.T) {
	step := NewNormalisationStep()
	suite := pipelinetesting.NewPipelineStepTestSuite(t, step, "testdata/normalisation")

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
