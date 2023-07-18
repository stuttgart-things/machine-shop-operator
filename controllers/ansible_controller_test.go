/*
Copyright 2023 patrick hermann.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controllers

import (
	"bytes"
	"html/template"
	"testing"
)

func TestCreateInventory(t *testing.T) {

	inventory := make(map[string][]string)
	inventoryString := []string{"all: localhost2", "master: rt.rancher.com;rt-2.rancher.com;rt-3.rancher.com", "worker: rt-4.rancher.com;rt-5.rancher.com"}

	for _, groups := range inventoryString {
		groupName, hosts := createInventoryValues(groups)
		inventory[groupName] = hosts
	}

	rendered := renderTemplate(inventory)

	if rendered != renderedInventory {
		t.Errorf("expected: %s\ngot: %s", renderedInventory, rendered)
	}

}

const inventoryTemplate = `{{ range $name, $value := . }}
[{{ $name }}]{{range $value }}
{{.}}{{end}}
{{ end }}`

const renderedInventory = `
[all]
localhost2

[master]
rt.rancher.com
rt-2.rancher.com
rt-3.rancher.com

[worker]
rt-4.rancher.com
rt-5.rancher.com
`

// var groupHosts = map[string][]string{
// 	"all":    {"localhost"},
// 	"master": {"rt.rancher.com", "rt-2.rancher.com", "rt-3.rancher.com"},
// 	"worker": {"rt-4.rancher.com", "rt-5.rancher.com"},
// }

// func createInventoryValues(inventoryString []string) (inventory map[string][]string) {

// 	inventory = make(map[string][]string)

// 	for _, groups := range inventoryString {
// 		groups := strings.Split(groups, ":")
// 		hosts := strings.Split(strings.TrimSpace(groups[1]), ";")
// 		inventory[strings.TrimSpace(groups[0])] = strings.Split(strings.TrimSpace(groups[1]), ";")

// 		fmt.Println("GROUP:", strings.TrimSpace(groups[0]))
// 		fmt.Println("HOSTS:", hosts)
// 	}

// 	return
// }

func renderTemplate(groupHosts map[string][]string) string {

	var buf bytes.Buffer

	t := template.New("template")
	t, err := t.Parse(inventoryTemplate)
	if err != nil {
		panic(err)
	}

	err = t.Execute(&buf, groupHosts)
	if err != nil {
		panic(err)
	}

	return buf.String()
}
