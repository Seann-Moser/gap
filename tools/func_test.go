package tools

import (
	"reflect"
	"testing"
)

// TestGetFunctionCalls tests the GetFunctionCalls function.
func TestGetFunctionCalls(t *testing.T) {
	tests := []struct {
		name          string
		functionCode  string
		functionInfo  FunctionInfo
		expectedCalls []FunctionCallInfo
	}{
		{
			name: "Simple function calls",
			functionCode: `
func simple() {
	Foo()
	Bar()
}
`,
			functionInfo: FunctionInfo{
				PkgName: "main",
				Name:    "simple",
			},
			expectedCalls: []FunctionCallInfo{
				{Function: "Foo"},
				{Function: "Bar"},
			},
		},
		{
			name: "Method calls with receiver",
			functionCode: `
func (s *MyStruct) method() {
    s.DoSomething()
    DoAnotherThing()
}
`,
			functionInfo: FunctionInfo{
				PkgName:    "main",
				Name:       "method",
				StructName: "*MyStruct",
			},
			expectedCalls: []FunctionCallInfo{
				{Function: "DoSomething", Receiver: "s"},
				{Function: "DoAnotherThing"},
			},
		},

		{
			name: "Nested function calls",
			functionCode: `
func nested() {
    result := Process(GetData(), Compute(42))
    Print(result)
}
`,
			functionInfo: FunctionInfo{
				PkgName: "main",
				Name:    "nested",
			},
			expectedCalls: []FunctionCallInfo{
				{
					Function:  "Process",
					Arguments: []string{"GetData()", "Compute(42)"},
					Calls: []FunctionCallInfo{
						{Function: "GetData", Arguments: []string{}},
						{Function: "Compute", Arguments: []string{"42"}},
					},
				},
				{
					Function:  "Print",
					Arguments: []string{"result"},
				},
			},
		},

		{
			name: "Function calls with package names",
			functionCode: `
func withPackages() {
	fmt.Println("Hello")
	math.Abs(-3.14)
}
`,
			functionInfo: FunctionInfo{
				PkgName: "main",
				Name:    "withPackages",
			},
			expectedCalls: []FunctionCallInfo{
				{Function: "Println", Package: "fmt"},
				{Function: "Abs", Package: "math"},
			},
		},
		{
			name: "Method calls with struct fields",
			functionCode: `
func (c *Controller) Handle() {
	c.Service.Execute()
}
`,
			functionInfo: FunctionInfo{
				PkgName:    "main",
				Name:       "Handle",
				StructName: "*Controller",
			},
			expectedCalls: []FunctionCallInfo{
				{Function: "Execute", Receiver: "c.Service"},
			},
		},
		{
			name: "Function literals (anonymous functions)",
			functionCode: `
func anonymous() {
	func() {
		InnerFunc()
	}()
}
`,
			functionInfo: FunctionInfo{
				PkgName: "main",
				Name:    "anonymous",
			},
			expectedCalls: []FunctionCallInfo{
				{Function: "func"}, // The anonymous function itself
				{Function: "InnerFunc"},
			},
		},
		{
			name: "Variadic function calls",
			functionCode: `
func variadic() {
	Log("Error:", "Something went wrong", "Code:", 500)
}
`,
			functionInfo: FunctionInfo{
				PkgName: "main",
				Name:    "variadic",
			},
			expectedCalls: []FunctionCallInfo{
				{
					Function:  "Log",
					Arguments: []string{`"Error:"`, `"Something went wrong"`, `"Code:"`, "500"},
				},
			},
		},
		{
			name: "Complex expressions",
			functionCode: `
func complex() {
	obj.Method().AnotherMethod().FinalMethod()
}
`,
			functionInfo: FunctionInfo{
				PkgName: "main",
				Name:    "complex",
			},
			expectedCalls: []FunctionCallInfo{
				{Function: "FinalMethod", Receiver: "obj.Method().AnotherMethod()"},
			},
		},
		{
			name: "Handling of defer and go statements",
			functionCode: `
func concurrency() {
	defer Cleanup()
	go RunTask()
}
`,
			functionInfo: FunctionInfo{
				PkgName: "main",
				Name:    "concurrency",
			},
			expectedCalls: []FunctionCallInfo{
				{Function: "Cleanup"},
				{Function: "RunTask"},
			},
		},
	}

	projectRoot := "" // Replace with the appropriate path if necessary
	filePath := ""

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fi := tt.functionInfo
			fi.RelativeFilePath = filePath

			calls, err := GetFunctionCalls(tt.functionCode, fi, projectRoot)
			if err != nil {
				t.Errorf("GetFunctionCalls() error = %v", err)
				return
			}

			// Normalize calls for comparison
			normalizeCalls(calls)
			normalizeCalls(tt.expectedCalls)

			if !reflect.DeepEqual(calls, tt.expectedCalls) {
				t.Errorf("GetFunctionCalls() = %v, want %v", calls, tt.expectedCalls)
			}
		})
	}
}

// normalizeCalls removes variable elements like line numbers and file paths for comparison.
func normalizeCalls(calls []FunctionCallInfo) {
	for i := range calls {
		calls[i].Line = 0
		calls[i].FilePath = ""
		calls[i].FullExpr = ""
		if len(calls[i].Calls) > 0 {
			normalizeCalls(calls[i].Calls)
		}
	}
}
