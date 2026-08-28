// Package handlers holds annotated example handlers. parse.ParseDir
// turns their @apidoc comments into doc registrations; the functions
// themselves are plain http.HandlerFuncs usable by net/http and chi
// directly and by gin/echo through their http.Handler wrappers.
package handlers

import (
	"net/http"
)

// User is a sample resource referenced by @param children below.
type User struct {
	ID    int64  `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

// ListUsers returns the user list.
//
// @apidoc
// @method GET
// @url /api/users
// @title 用户列表
// @param page int query false "页码" default=1
// @success ok []User "用户列表"
func ListUsers(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`[{"id":1,"name":"erik","email":"erik@example.com"}]`))
}

// GetUser returns one user.
//
// @apidoc
// @method GET
// @url /api/users/:id
// @title 用户详情
// @param id int64 true "用户ID" in=path
// @success ok User "用户信息"
func GetUser(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"id":1,"name":"erik","email":"erik@example.com"}`))
}

// CreateUser creates a user.
//
// @apidoc
// @method POST
// @url /api/users
// @title 创建用户
// @param body object body true "用户信息" children=User
// @success ok string "创建结果"
func CreateUser(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusCreated)
	_, _ = w.Write([]byte(`ok`))
}
