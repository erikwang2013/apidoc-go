// Package model holds the internal documentation data model.
// The whole model is JSON-serializable so a future go/ast parser can
// populate the same store that runtime registration fills today.
package model

// Project is the root of the doc tree.
type Project struct {
	Name string `json:"name"`
	Apps []*App `json:"apps"`
}

// App is a named application (multi-app support).
type App struct {
	Key      string     `json:"key"`
	Title    string     `json:"title"`
	Versions []*Version `json:"versions"`
}

// Version groups controllers under an app.
type Version struct {
	Name        string        `json:"name"`
	Controllers []*Controller `json:"controllers"`
}

// Controller groups actions (the apidoc-php "group" concept).
type Controller struct {
	Name    string    `json:"name"`
	Actions []*Action `json:"actions"`
}

// Action is one documented endpoint.
type Action struct {
	ID         string     `json:"id"`
	App        string     `json:"app"`
	Version    string     `json:"version"`
	Controller string     `json:"controller"`
	Method     string     `json:"method"`
	URL        string     `json:"url"`
	Title      string     `json:"title"`
	Desc       string     `json:"desc"`
	Author     string     `json:"author"`
	Params     []Param    `json:"params"`
	Responses  []Response `json:"responses"`
	Markdown   string     `json:"markdown,omitempty"`
	Mock       string     `json:"mock,omitempty"`
	Sort       int        `json:"sort"`
}

// Param describes one input of an action.
type Param struct {
	Name     string  `json:"name"`
	In       string  `json:"in"` // header|query|body|path
	Type     string  `json:"type"`
	Desc     string  `json:"desc"`
	Required bool    `json:"required"`
	Default  string  `json:"default,omitempty"`
	Mock     string  `json:"mock,omitempty"`
	Children []Param `json:"children,omitempty"`
}

// Response describes one possible response of an action.
type Response struct {
	Name   string  `json:"name"`
	Type   string  `json:"type"`
	Desc   string  `json:"desc"`
	Mock   string  `json:"mock,omitempty"`
	Fields []Param `json:"fields,omitempty"`
}

// Doc is a registration record: the user-facing Doc plus the tree
// position defaults are normalized before it hits the store.
type Doc struct {
	App, Version, Controller string
	Title, Desc, Author      string
	Params                   []Param
	Responses                []Response
	Markdown                 string
	Mock                     string
	Sort                     int
}
