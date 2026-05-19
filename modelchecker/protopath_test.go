package modelchecker

import (
	ast "fizz/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"
	"sync"
	"testing"
)

func TestGetProtoFieldByPath(t *testing.T) {
	file, err := readFileToAst()
	require.Nil(t, err)
	msg := GetProtoFieldByPath(file, "Actions[0]")
	assert.NotNil(t, msg)
	assert.IsType(t, &ast.Action{}, msg)

	msg = GetProtoFieldByPath(file, "Actions[0].Block")
	assert.NotNil(t, msg)
	assert.IsType(t, &ast.Block{}, msg)

	msg = GetProtoFieldByPath(file, "Actions[0].Block.Stmts[0]")
	assert.NotNil(t, msg)
	assert.IsType(t, &ast.Statement{}, msg)

	msg = GetProtoFieldByPath(file, "Actions[0].Block.Stmts[0].AnyStmt")
	assert.NotNil(t, msg)
	assert.IsType(t, &ast.AnyStmt{}, msg)

	msg = GetProtoFieldByPath(file, "Actions[0].Block.Stmts[0].AnyStmt.Block")
	assert.NotNil(t, msg)
	assert.IsType(t, &ast.Block{}, msg)

	msg = GetProtoFieldByPath(file, "Actions[0].Block.Stmts[0].AnyStmt.Block.Stmts[0].IfStmt")
	assert.NotNil(t, msg)
	assert.IsType(t, &ast.IfStmt{}, msg)

	msg = GetProtoFieldByPath(file, "Actions[0].Block.Stmts[0].AnyStmt.Block.Stmts[0].IfStmt.Branches[0].Block")
	assert.NotNil(t, msg)
	assert.IsType(t, &ast.Block{}, msg)

	msg = GetProtoFieldByPath(file, "Actions[0].Block.Stmts[0].AnyStmt.Block.Stmts[0].IfStmt.Branches[0].Block.Stmts[0]")
	assert.NotNil(t, msg)
	assert.IsType(t, &ast.Statement{}, msg)

	msg = GetProtoFieldByPath(file, "Actions[0].Block.Stmts[0].AnyStmt.Block.Stmts[0].IfStmt.Branches[0].Block.Stmts[0].PyStmt")
	assert.NotNil(t, msg)
	assert.IsType(t, &ast.PyStmt{}, msg)

	field, valid := GetStringFieldByPath(file, "Actions[0].Block.Stmts[0].AnyStmt.Block.Stmts[0].IfStmt.Branches[0].Block.Stmts[0].PyStmt.Code")
	assert.True(t, valid)
	assert.NotNil(t, field)
	assert.IsType(t, " ", field)
	assert.Equal(t, "elements = elements | set([e])", field)

	field, valid = GetStringFieldByPath(file, "Actions[0].Block.Stmts[0].AnyStmt.Block.Stmts[0].IfStmt.Branches[0].Block.Stmts[1].PyStmt.Code")
	assert.True(t, valid)
	assert.NotNil(t, field)
	assert.IsType(t, " ", field)
	assert.Equal(t, "count = count + 1", field)
}

func TestGetNextPath(t *testing.T) {
	file, err := readFileToAst()
	require.Nil(t, err)
	path, v := GetNextFieldPath(file, "Actions[0].Block.Stmts[0].AnyStmt.Block.Stmts[0].IfStmt.Branches[0].Block.Stmts[0].PyStmt.Code")
	assert.NotNil(t, v)
	assert.Equal(t, "Actions[0].Block.Stmts[0].AnyStmt.Block.Stmts[0].IfStmt.Branches[0].Block.Stmts[1]", path)

	path, v = GetNextFieldPath(file, path)
	assert.Nil(t, v)
	assert.Equal(t, "Actions[0].Block.Stmts[0].AnyStmt.Block.Stmts[0].IfStmt.Branches[0].Block.$", path)
}

func TestEndOfBlock(t *testing.T) {
	assert.Equal(t, "Actions[0].Block.Stmts[0].AnyStmt.Block.Stmts[0].IfStmt.Branches[0].Block.$",
		EndOfBlock("Actions[0].Block.Stmts[0].AnyStmt.Block.Stmts[0].IfStmt.Branches[0].Block.Stmts[0]"))
}

// TestGetProtoFieldByPath_Concurrent exercises the cache under parallel reads
// + writes. Run with `go test -race` to catch any locking regression. The
// shared protoPathInstance cache is hit by every Processor; parallel
// simulation workers must not race on it.
func TestGetProtoFieldByPath_Concurrent(t *testing.T) {
	file, err := readFileToAst()
	require.Nil(t, err)

	paths := []string{
		"Actions[0]",
		"Actions[0].Block",
		"Actions[0].Block.Stmts[0]",
		"Actions[0].Block.Stmts[0].AnyStmt",
		"Actions[0].Block.Stmts[0].AnyStmt.Block",
		"Actions[1]",
		"Actions[1].Block",
		"Actions[1].Block.Stmts[0]",
		"Actions[1].Block.Stmts[0].AnyStmt",
		"NonExistentPath[42]", // exercises the nil-result cache entry
	}

	const workers = 16
	const iterations = 200

	var wg sync.WaitGroup
	wg.Add(workers)
	for w := 0; w < workers; w++ {
		go func() {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				for _, p := range paths {
					_ = GetProtoFieldByPath(file, p)
				}
			}
		}()
	}
	wg.Wait()
}

func readFileToAst() (*ast.File, error) {
	jsonFile := `
{
  "states": {
    "code": "MAX_ELEMENTS = 5\nelements = set()\ncount = 0"
  },
  "actions": [
    {
      "name": "add",
      "block": {
        "flow": "FLOW_ATOMIC",
        "stmts": [
          {
            "anyStmt": {
              "loop_vars": [
                "e"
              ],
              "pyExpr": "range(0, MAX_ELEMENTS)",
              "block": {
                "flow": "FLOW_ATOMIC",
                "stmts": [
                  {
                    "ifStmt": {
                      "branches": [
                        {
                          "condition": "e not in elements",
                          "block": {
                            "flow": "FLOW_ATOMIC",
                            "stmts": [
                              {
                                "pyStmt": {
                                  "code": "elements = elements | set([e])"
                                }
                              },
                              {
                                "pyStmt": {
                                  "code": "count = count + 1"
                                }
                              }
                            ]
                          }
                        }
                      ]
                    }
                  }
                ]
              }
            }
          }
        ]
      }
    },
    {
      "name": "remove",
      "block": {
        "flow": "FLOW_ATOMIC",
        "stmts": [
          {
            "anyStmt": {
              "loop_vars": [
                "e"
              ],
              "pyExpr": "elements",
              "block": {
                "flow": "FLOW_ATOMIC",
                "stmts": [
                  {
                    "pyStmt": {
                      "code": "elements = elements - set([e])"
                    }
                  },
                  {
                    "pyStmt": {
                      "code": "count = count - 1"
                    }
                  }
                ]
              }
            }
          }
        ]
      }
    }
  ]
}
`

	return parseAstFromString(jsonFile)
}

func parseAstFromString(jsonFile string) (*ast.File, error) {
	f := &ast.File{}
	err := protojson.Unmarshal([]byte(jsonFile), f)

	return f, err
}
