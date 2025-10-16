/*
Copyright 2019 The Contributors.

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

package sinks

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	rockset "github.com/rockset/rockset-go-client"
	v1 "k8s.io/api/core/v1"
)

/*
RocksetSink is a sink that uploads the kubernetes events as json object
and converts them to documents inside of a Rockset collection.

Rockset can later be used with
many different connectors such as Tableau or Redash to use this data.
*/
type RocksetSink struct {
	client                *rockset.RockClient
	rocksetCollectionName string
	rocksetWorkspaceName  string
}

// NewRocksetSink will create a new RocksetSink with default options, returned as
// an EventSinkInterface
func NewRocksetSink(rocksetAPIKey string, rocksetCollectionName string, rocksetWorkspaceName string) EventSinkInterface {
	client, err := rockset.NewClient(rockset.WithAPIKey(rocksetAPIKey))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create Rockset client: %v\n", err)
		return nil
	}
	return &RocksetSink{
		client:                client,
		rocksetCollectionName: rocksetCollectionName,
		rocksetWorkspaceName:  rocksetWorkspaceName,
	}
}

// UpdateEvents implements the EventSinkInterface
func (rs *RocksetSink) UpdateEvents(eNew *v1.Event, eOld *v1.Event) {
	eData := NewEventData(eNew, eOld)

	if eJSONBytes, err := json.Marshal(eData); err == nil {
		var m map[string]interface{}
		json.Unmarshal(eJSONBytes, &m)
		docs := []interface{}{
			m,
		}
		ctx := context.Background()
		_, err := rs.client.AddDocuments(ctx, rs.rocksetWorkspaceName, rs.rocksetCollectionName, docs)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to add document to Rockset: %v\n", err)
		}
	} else {
		fmt.Fprintf(os.Stderr, "Failed to json serialize event: %v\n", err)
	}
}
