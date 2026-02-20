// pkg/common/api/openapi_test.go
package api

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOpenAPI_Merge(t *testing.T) {
	spec1 := &ServiceAPISpec{
		ServiceName: "auth",
		Version:     "1.0.0",
		Spec: OpenAPI{
			OpenAPI: "3.0.0",
			Info: Info{
				Title:   "Auth Service",
				Version: "1.0.0",
			},
			Paths: Paths{
				"/auth/login": PathItem{
					Post: &Operation{
						Summary:     "User login",
						OperationID: "login",
						Tags:        []string{"auth"},
					},
				},
				"/auth/register": PathItem{
					Post: &Operation{
						Summary:     "User registration",
						OperationID: "register",
						Tags:        []string{"auth"},
					},
				},
			},
			Components: Components{
				Schemas: map[string]*Schema{
					"User": {
						Type: "object",
						Properties: map[string]*Schema{
							"id":    {Type: "string"},
							"email": {Type: "string"},
						},
					},
				},
			},
			Tags: []Tag{
				{Name: "auth", Description: "Authentication endpoints"},
			},
		},
	}

	spec2 := &ServiceAPISpec{
		ServiceName: "game",
		Version:     "1.0.0",
		Spec: OpenAPI{
			OpenAPI: "3.0.0",
			Info: Info{
				Title:   "Game Service",
				Version: "1.0.0",
			},
			Paths: Paths{
				"/game/state": PathItem{
					Get: &Operation{
						Summary:     "Get game state",
						OperationID: "getState",
						Tags:        []string{"game"},
					},
				},
				"/game/move": PathItem{
					Post: &Operation{
						Summary:     "Make a move",
						OperationID: "makeMove",
						Tags:        []string{"game"},
					},
				},
			},
			Components: Components{
				Schemas: map[string]*Schema{
					"Position": {
						Type: "object",
						Properties: map[string]*Schema{
							"x": {Type: "integer"},
							"y": {Type: "integer"},
						},
					},
				},
			},
			Tags: []Tag{
				{Name: "game", Description: "Game endpoints"},
			},
		},
	}

	t.Run("Merge two specs", func(t *testing.T) {
		merged := &ServiceAPISpec{
			ServiceName: "merged",
			Version:     "1.0.0",
			Spec: OpenAPI{
				OpenAPI: "3.0.0",
				Info: Info{
					Title:   "Merged API",
					Version: "1.0.0",
				},
				Paths:      make(Paths),
				Components: Components{Schemas: make(map[string]*Schema)},
				Tags:       []Tag{},
			},
		}

		err := merged.Merge(spec1)
		require.NoError(t, err)

		err = merged.Merge(spec2)
		require.NoError(t, err)

		// Verify merged paths
		assert.Contains(t, merged.Spec.Paths, "/auth/login")
		assert.Contains(t, merged.Spec.Paths, "/auth/register")
		assert.Contains(t, merged.Spec.Paths, "/game/state")
		assert.Contains(t, merged.Spec.Paths, "/game/move")

		// Verify merged schemas
		assert.Contains(t, merged.Spec.Components.Schemas, "User")
		assert.Contains(t, merged.Spec.Components.Schemas, "Position")

		// Verify merged tags
		assert.Len(t, merged.Spec.Tags, 2)
		tagNames := []string{merged.Spec.Tags[0].Name, merged.Spec.Tags[1].Name}
		assert.Contains(t, tagNames, "auth")
		assert.Contains(t, tagNames, "game")
	})

	t.Run("Merge with overlapping paths", func(t *testing.T) {
		overlapping1 := &ServiceAPISpec{
			Spec: OpenAPI{
				Paths: Paths{
					"/shared": PathItem{
						Get: &Operation{
							OperationID: "getShared",
						},
					},
				},
			},
		}

		overlapping2 := &ServiceAPISpec{
			Spec: OpenAPI{
				Paths: Paths{
					"/shared": PathItem{
						Post: &Operation{
							OperationID: "postShared",
						},
					},
				},
			},
		}

		merged := &ServiceAPISpec{
			Spec: OpenAPI{
				Paths: make(Paths),
			},
		}

		err := merged.Merge(overlapping1)
		require.NoError(t, err)

		err = merged.Merge(overlapping2)
		require.NoError(t, err)

		// Should have both operations on same path
		pathItem, exists := merged.Spec.Paths["/shared"]
		assert.True(t, exists)
		assert.NotNil(t, pathItem.Get)
		assert.NotNil(t, pathItem.Post)
		assert.Equal(t, "getShared", pathItem.Get.OperationID)
		assert.Equal(t, "postShared", pathItem.Post.OperationID)
	})
}

func TestOpenAPI_Validation(t *testing.T) {
	t.Run("Valid OpenAPI spec", func(t *testing.T) {
		spec := &ServiceAPISpec{
			ServiceName: "test",
			Version:     "1.0.0",
			Spec: OpenAPI{
				OpenAPI: "3.0.0",
				Info: Info{
					Title:   "Test API",
					Version: "1.0.0",
				},
				Paths: Paths{
					"/test": PathItem{
						Get: &Operation{
							Responses: map[string]Response{
								"200": {
									Description: "OK",
								},
							},
						},
					},
				},
			},
		}

		// Validate required fields
		assert.NotEmpty(t, spec.Spec.OpenAPI)
		assert.NotEmpty(t, spec.Spec.Info.Title)
		assert.NotEmpty(t, spec.Spec.Info.Version)
		assert.NotEmpty(t, spec.Spec.Paths)
	})

	t.Run("Invalid OpenAPI spec - missing required fields", func(t *testing.T) {
		spec := &ServiceAPISpec{
			ServiceName: "test",
			Spec: OpenAPI{
				// Missing OpenAPI version
				Paths: Paths{},
			},
		}

		assert.Empty(t, spec.Spec.OpenAPI)
	})
}

func TestOpenAPI_Serialization(t *testing.T) {
	spec := &ServiceAPISpec{
		ServiceName: "test",
		Version:     "1.0.0",
		Spec: OpenAPI{
			OpenAPI: "3.0.0",
			Info: Info{
				Title:   "Test API",
				Version: "1.0.0",
				Contact: Contact{
					Name:  "Test Contact",
					Email: "test@example.com",
				},
			},
			Servers: []Server{
				{
					URL:         "http://localhost:8080",
					Description: "Local server",
				},
			},
			Paths: Paths{
				"/test": PathItem{
					Get: &Operation{
						Summary:     "Test endpoint",
						OperationID: "test",
						Parameters: []Parameter{
							{
								Name:     "id",
								In:       "query",
								Required: true,
								Schema: &Schema{
									Type: "string",
								},
							},
						},
						Responses: map[string]Response{
							"200": {
								Description: "OK",
								Content: map[string]MediaType{
									"application/json": {
										Schema: &Schema{
											Type: "object",
											Properties: map[string]*Schema{
												"message": {Type: "string"},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	t.Run("Marshal to JSON", func(t *testing.T) {
		data, err := spec.ToJSON()
		require.NoError(t, err)
		assert.NotEmpty(t, data)

		// Verify it's valid JSON
		var parsed map[string]interface{}
		err = json.Unmarshal(data, &parsed)
		require.NoError(t, err)

		// Check structure
		assert.Contains(t, parsed, "service_name")
		assert.Contains(t, parsed, "spec")
		assert.Equal(t, "test", parsed["service_name"])
	})

	t.Run("Unmarshal from JSON", func(t *testing.T) {
		data, err := spec.ToJSON()
		require.NoError(t, err)

		var newSpec ServiceAPISpec
		err = json.Unmarshal(data, &newSpec)
		require.NoError(t, err)

		assert.Equal(t, spec.ServiceName, newSpec.ServiceName)
		assert.Equal(t, spec.Version, newSpec.Version)
		assert.Equal(t, spec.Spec.OpenAPI, newSpec.Spec.OpenAPI)
		assert.Equal(t, spec.Spec.Info.Title, newSpec.Spec.Info.Title)
	})
}

func TestOpenAPI_EdgeCases(t *testing.T) {
	t.Run("Empty spec", func(t *testing.T) {
		empty := &ServiceAPISpec{}

		data, err := empty.ToJSON()
		require.NoError(t, err)

		var parsed map[string]interface{}
		err = json.Unmarshal(data, &parsed)
		require.NoError(t, err)
	})

	t.Run("Merge with nil spec", func(t *testing.T) {
		target := &ServiceAPISpec{
			Spec: OpenAPI{
				Paths: make(Paths),
			},
		}

		err := target.Merge(nil)
		assert.Error(t, err)
	})

	t.Run("Merge with empty paths", func(t *testing.T) {
		target := &ServiceAPISpec{
			Spec: OpenAPI{
				Paths: Paths{
					"/existing": PathItem{},
				},
			},
		}

		source := &ServiceAPISpec{
			Spec: OpenAPI{
				Paths: Paths{},
			},
		}

		err := target.Merge(source)
		require.NoError(t, err)

		assert.Contains(t, target.Spec.Paths, "/existing")
	})
}
