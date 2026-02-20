// pkg/common/api/openapi.go
package api

import (
	"encoding/json"
	"time"
)

type OpenAPI struct {
	OpenAPI    string     `json:"openapi"`
	Info       Info       `json:"info"`
	Servers    []Server   `json:"servers"`
	Paths      Paths      `json:"paths"`
	Components Components `json:"components"`
	Tags       []Tag      `json:"tags"`
}

type Info struct {
	Title       string  `json:"title"`
	Description string  `json:"description"`
	Version     string  `json:"version"`
	Contact     Contact `json:"contact,omitempty"`
}

type Contact struct {
	Name  string `json:"name,omitempty"`
	Email string `json:"email,omitempty"`
	URL   string `json:"url,omitempty"`
}

type Server struct {
	URL         string              `json:"url"`
	Description string              `json:"description"`
	Variables   map[string]Variable `json:"variables,omitempty"`
}

type Variable struct {
	Default string   `json:"default"`
	Enum    []string `json:"enum,omitempty"`
}

type Paths map[string]PathItem

type PathItem struct {
	Get     *Operation `json:"get,omitempty"`
	Post    *Operation `json:"post,omitempty"`
	Put     *Operation `json:"put,omitempty"`
	Delete  *Operation `json:"delete,omitempty"`
	Patch   *Operation `json:"patch,omitempty"`
	Head    *Operation `json:"head,omitempty"`
	Options *Operation `json:"options,omitempty"`
	Trace   *Operation `json:"trace,omitempty"`
}

type Operation struct {
	Tags        []string              `json:"tags,omitempty"`
	Summary     string                `json:"summary,omitempty"`
	Description string                `json:"description,omitempty"`
	OperationID string                `json:"operationId"`
	Parameters  []Parameter           `json:"parameters,omitempty"`
	RequestBody *RequestBody          `json:"requestBody,omitempty"`
	Responses   map[string]Response   `json:"responses"`
	Security    []map[string][]string `json:"security,omitempty"`
}

type Parameter struct {
	Name            string  `json:"name"`
	In              string  `json:"in"`
	Description     string  `json:"description,omitempty"`
	Required        bool    `json:"required,omitempty"`
	Schema          *Schema `json:"schema,omitempty"`
	AllowEmptyValue bool    `json:"allowEmptyValue,omitempty"`
}

type RequestBody struct {
	Description string               `json:"description,omitempty"`
	Required    bool                 `json:"required"`
	Content     map[string]MediaType `json:"content"`
}

type MediaType struct {
	Schema   *Schema            `json:"schema,omitempty"`
	Example  interface{}        `json:"example,omitempty"`
	Examples map[string]Example `json:"examples,omitempty"`
}

type Response struct {
	Description string               `json:"description"`
	Content     map[string]MediaType `json:"content,omitempty"`
}

type Example struct {
	Summary       string      `json:"summary,omitempty"`
	Description   string      `json:"description,omitempty"`
	Value         interface{} `json:"value"`
	ExternalValue string      `json:"externalValue,omitempty"`
}

type Schema struct {
	Type                 string             `json:"type,omitempty"`
	Format               string             `json:"format,omitempty"`
	Description          string             `json:"description,omitempty"`
	Properties           map[string]*Schema `json:"properties,omitempty"`
	Required             []string           `json:"required,omitempty"`
	Items                *Schema            `json:"items,omitempty"`
	Ref                  string             `json:"$ref,omitempty"`
	AdditionalProperties interface{}        `json:"additionalProperties,omitempty"`
	Enum                 []interface{}      `json:"enum,omitempty"`
	Default              interface{}        `json:"default,omitempty"`
	Example              interface{}        `json:"example,omitempty"`
}

type Components struct {
	Schemas         map[string]*Schema        `json:"schemas,omitempty"`
	SecuritySchemes map[string]SecurityScheme `json:"securitySchemes,omitempty"`
}

type SecurityScheme struct {
	Type             string      `json:"type"`
	Description      string      `json:"description,omitempty"`
	Name             string      `json:"name,omitempty"`
	In               string      `json:"in,omitempty"`
	Scheme           string      `json:"scheme,omitempty"`
	BearerFormat     string      `json:"bearerFormat,omitempty"`
	Flows            *OAuthFlows `json:"flows,omitempty"`
	OpenIdConnectUrl string      `json:"openIdConnectUrl,omitempty"`
}

type OAuthFlows struct {
	Implicit          *OAuthFlow `json:"implicit,omitempty"`
	Password          *OAuthFlow `json:"password,omitempty"`
	ClientCredentials *OAuthFlow `json:"clientCredentials,omitempty"`
	AuthorizationCode *OAuthFlow `json:"authorizationCode,omitempty"`
}

type OAuthFlow struct {
	AuthorizationUrl string            `json:"authorizationUrl,omitempty"`
	TokenUrl         string            `json:"tokenUrl,omitempty"`
	RefreshUrl       string            `json:"refreshUrl,omitempty"`
	Scopes           map[string]string `json:"scopes"`
}

type Tag struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

type ServiceAPISpec struct {
	ServiceName string    `json:"service_name"`
	ServiceID   string    `json:"service_id"`
	Version     string    `json:"version"`
	Spec        OpenAPI   `json:"spec"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func (s *ServiceAPISpec) Merge(other *ServiceAPISpec) error {
	// Merge paths
	for path, item := range other.Spec.Paths {
		if existing, ok := s.Spec.Paths[path]; ok {
			// Merge operations
			if item.Get != nil {
				existing.Get = item.Get
			}
			if item.Post != nil {
				existing.Post = item.Post
			}
			if item.Put != nil {
				existing.Put = item.Put
			}
			if item.Delete != nil {
				existing.Delete = item.Delete
			}
			s.Spec.Paths[path] = existing
		} else {
			s.Spec.Paths[path] = item
		}
	}

	// Merge schemas
	for name, schema := range other.Spec.Components.Schemas {
		s.Spec.Components.Schemas[name] = schema
	}

	// Merge tags
	tagMap := make(map[string]bool)
	for _, tag := range s.Spec.Tags {
		tagMap[tag.Name] = true
	}
	for _, tag := range other.Spec.Tags {
		if !tagMap[tag.Name] {
			s.Spec.Tags = append(s.Spec.Tags, tag)
		}
	}

	return nil
}

func (s *ServiceAPISpec) ToJSON() ([]byte, error) {
	return json.MarshalIndent(s, "", "  ")
}
