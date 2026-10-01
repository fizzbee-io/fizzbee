package modelchecker

const (
	ActionsWithMultipleBlocks = `
{
  "states": {
    "code": "a=0\nb=0"
  },
  "actions": [
    {
      "name": "FirstAction",
      "block": {
        "flow": "FLOW_ATOMIC",
        "stmts": [
          {
            "pyStmt": {
              "code": "a = a + 1"
            }   
          },
          {
            "pyStmt": {
              "code": "b = b + 2"
            }
          },
          {
            "block": {
              "flow": "FLOW_ATOMIC",
              "stmts": [
                {
                  "pyStmt": {
                    "code": "a = a + 1"
                  }   
                },
                {
                  "pyStmt": {
                    "code": "b = b + 2"
                  }
                }
              ]
            }
          },
          {
            "pyStmt": {
              "code": "a = a + 1"
            }   
          },
          {
            "pyStmt": {
              "code": "b = b + 2"
            }
          }
        ]
      }
    },
    {
      "name": "SecondAction",
      "block": {
        "flow": "FLOW_ONEOF",
        "stmts": [
          {
            "pyStmt": {
              "code": "a = a + 1"
            }   
          },
          {
            "pyStmt": {
              "code": "b = b + 2"
            }
          },
          {
            "block": {
              "flow": "FLOW_ONEOF",
              "stmts": [
                {
                  "pyStmt": {
                    "code": "a = a + 1"
                  }   
                },
                {
                  "pyStmt": {
                    "code": "b = b + 2"
                  }
                }
              ]
            }
          },
          {
            "pyStmt": {
              "code": "a = a + 1"
            }   
          },
          {
            "pyStmt": {
              "code": "b = b + 2"
            }
          }
        ]
      }
    },
    {
      "name": "ThirdAction",
      "block": {
        "flow": "FLOW_SERIAL",
        "stmts": [
          {
            "pyStmt": {
              "code": "a = a + 1"
            }   
          },
          {
            "pyStmt": {
              "code": "b = b + 2"
            }
          },
          {
            "block": {
              "flow": "FLOW_SERIAL",
              "stmts": [
                {
                  "pyStmt": {
                    "code": "a = a + 1"
                  }   
                },
                {
                  "pyStmt": {
                    "code": "b = b + 2"
                  }
                }
              ]
            }
          },
          {
            "pyStmt": {
              "code": "a = a + 1"
            }   
          },
          {
            "pyStmt": {
              "code": "b = b + 2"
            }
          }
        ]
      }
    },
    {
      "name": "ThirdAction",
      "block": {
        "flow": "FLOW_PARALLEL",
        "stmts": [
          {
            "pyStmt": {
              "code": "a = a + 1"
            }   
          },
          {
            "pyStmt": {
              "code": "b = b + 2"
            }
          },
          {
            "block": {
              "flow": "FLOW_PARALLEL",
              "stmts": [
                {
                  "pyStmt": {
                    "code": "a = a + 1"
                  }   
                },
                {
                  "pyStmt": {
                    "code": "b = b + 2"
                  }
                }
              ]
            }
          },
          {
            "pyStmt": {
              "code": "a = a + 1"
            }   
          },
          {
            "pyStmt": {
              "code": "b = b + 2"
            }
          }
        ]
      }
    }
  ]
}
`
	RequireInParallelBlock = `
{
  "states": {
    "code": "ready=False\na=0"
  },
  "actions": [
    {
      "name": "Both",
      "flow": "FLOW_PARALLEL",
      "block": {
        "flow": "FLOW_PARALLEL",
        "stmts": [
          {
            "requireStmt": {
              "sourceInfo": {
                "start": {
                  "line": 9,
                  "column": 5
                }
              },
              "condition": "ready",
              "conditionExpr": {
                "pyExpr": "ready"
              }
            }
          },
          {
            "pyStmt": {
              "code": "a = a + 1"
            }
          }
        ]
      }
    }
  ]
}
`
)
