package tests

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"

	"github.com/google/go-cmp/cmp"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/testing/protocmp"

	"github.com/ydb-platform/ydbops/pkg/client/cms"
	"github.com/ydb-platform/ydbops/pkg/options"
	blackmagic "github.com/ydb-platform/ydbops/tests/black-magic"
	"github.com/ydb-platform/ydbops/tests/mock"
)

const (
	uuidRegexpString       = "[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}"
	testWillInsertTaskUuid = "test_will_substitute_this_arg_with_task_uuid"
)

type Command []string

type StepData struct {
	expectedRequests      []proto.Message
	expectedOutputRegexps []string
	ydbopsInvocation      Command
}

type TestCase struct {
	nodeConfiguration [][]uint32
	nodeInfoMap       map[uint32]mock.TestNodeInfo

	steps                   []StepData
	additionalMockBehaviour *mock.AdditionalMockBehaviour
}

var (
	ydb              *mock.YdbMock
	previousEnvVars  map[string]string
	commonYdbopsArgs Command
)

func prepareEnvVariables() map[string]string {
	previous := make(map[string]string)

	newValue := mock.TestPassword
	os.Setenv(options.DefaultStaticPasswordEnvVar, newValue)
	previous[options.DefaultStaticPasswordEnvVar] = os.Getenv(options.DefaultStaticPasswordEnvVar)

	return previous
}

func revertEnvVariables(previous map[string]string) {
	for k, v := range previous {
		os.Setenv(k, v)
	}
}

func RunBeforeEach() {
	port := 2135
	ydb = mock.NewYdbMockServer()
	ydb.SetupSimpleTLS(
		filepath.Join(".", "test-data", "ssl-data", "ca.crt"),
		filepath.Join(".", "test-data", "ssl-data", "ca_unencrypted.key"),
	)
	ydb.StartOn(port)

	previousEnvVars = prepareEnvVariables()
}

func RunAfterEach() {
	ydb.Teardown()
	revertEnvVariables(previousEnvVars)
}

func RunTestCase(tc TestCase) {
	ydb.SetNodeConfiguration(tc.nodeConfiguration, tc.nodeInfoMap)

	if tc.additionalMockBehaviour == nil {
		tc.additionalMockBehaviour = &mock.AdditionalMockBehaviour{}
	}
	ydb.SetMockBehaviour(*tc.additionalMockBehaviour)

	var maintenanceTaskId string
	for _, step := range tc.steps {
		commandArgs := step.ydbopsInvocation

		for i, arg := range commandArgs {
			if arg == testWillInsertTaskUuid {
				// `maintenanceTaskId` is guaranteed to be valid at this point.
				// Look for the commentary where `maintenanceTaskId` is populated.
				commandArgs[i] = maintenanceTaskId
			}
		}

		cmd := exec.Command(filepath.Join("..", "ydbops"), commandArgs...)
		outputBytes, _ := cmd.CombinedOutput()
		// TODO some tests return with an error. Maybe tune this test a bit
		// so it includes checking the error code as well
		// Expect(err).To(BeNil())
		output := string(outputBytes)

		for _, expectedOutputRegexp := range step.expectedOutputRegexps {
			// This `if` means that `ydbops maintenance create` command has just
			// finished executing. We will extract maintenance task id from it
			// and pass it to the next invocations within this test.
			if strings.Contains(expectedOutputRegexp, "Your task id is:") {
				uuidOnlyRegexp := regexp.MustCompile(
					fmt.Sprintf("(%s%s)",
						cms.TaskUuidPrefix,
						uuidRegexpString,
					),
				)
				maintenanceTaskId = uuidOnlyRegexp.FindString(output)
			}

			r := regexp.MustCompile(expectedOutputRegexp)
			pos := r.FindStringIndex(output)
			if pos == nil {
				Fail(fmt.Sprintf(
					"The required pattern was not found in output.\nPattern:\n%s\nOutput:\n%s",
					expectedOutputRegexp,
					output,
				))
			}
			output = output[pos[1]:]
		}

		actualRequests := ydb.RequestLog
		// Cleanup protobuf log for next step in the test:
		ydb.RequestLog = []proto.Message{}

		// for _, req := range actualRequests {
		// 	fmt.Printf("\n%+v : %+v\n", reflect.TypeOf(req), req)
		// }

		// It is much easier to remove OperationParams field (generated by cms client) than to
		// teach our checker to ignore this field when comparing with mocked answers.
		for _, actualReq := range actualRequests {
			field := reflect.ValueOf(actualReq).Elem().FieldByName("OperationParams")
			if field.IsValid() {
				field.Set(reflect.Zero(field.Type()))
			}
		}

		defer func() {
			if r := recover(); r != nil {
				if strings.Contains(fmt.Sprintf("%v", r), "non-deterministic or non-symmetric function detected") {
					Fail(`UuidComparer failed, see logs for more info.`)
				} else {
					panic(r)
				}
			}
		}()

		expectedPlaceholders := make(map[string]int)
		actualPlaceholders := make(map[string]int)

		Expect(len(step.expectedRequests)).To(Equal(len(actualRequests)))

		for i, expected := range step.expectedRequests {
			actual := actualRequests[i]
			Expect(cmp.Diff(expected, actual,
				protocmp.Transform(),
				blackmagic.ActionGroupSorter(),
				blackmagic.UUIDComparer(expectedPlaceholders, actualPlaceholders),
			)).To(BeEmpty())
		}
	}
}
