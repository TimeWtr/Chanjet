// Copyright 2025 TimeWtr
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"os"
	"path/filepath"
	"text/template"
)

type TypeDef struct {
	Name         string // type name, such as Int64, Int32
	Base         string // basic name, such as int64, int32
	AtomicSuffix string // atomic package suffix, such as Int64, Int32
	IsSigned     bool   // is unsigned
}

func main() {
	types := []TypeDef{
		{"Int32", "int32", "Int32", true},
		{"Int64", "int64", "Int64", true},
		{"Uint32", "uint32", "Uint32", false},
		{"Uint64", "uint64", "Uint64", false},
		{"Uintptr", "uintptr", "Uintptr", false},
	}

	pwd, _ := os.Getwd()
	basePath := filepath.Join(pwd, "utils", "atomicx")
	tmpl := template.Must(template.ParseFiles(filepath.Join(basePath, "gen", "atomic_generated.go.tmpl")))
	f, _ := os.Create(filepath.Join(basePath, "atomic_generated.go"))
	defer func(f *os.File) {
		_ = f.Close()
	}(f)

	if err := tmpl.Execute(f, struct{ Types []TypeDef }{types}); err != nil {
		panic(err)
	}
}
