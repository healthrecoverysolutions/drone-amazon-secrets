// Copyright 2018 Drone.IO Inc
// Use of this software is governed by the Business Source License
// that can be found in the LICENSE file.

package plugin

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/sirupsen/logrus"

	"github.com/drone/drone-go/drone"
	"github.com/drone/drone-go/plugin/secret"

	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/aws/aws-sdk-go/aws"
)

// New returns a new secret plugin that sources secrets
// from the AWS secrets manager.
func New(manager *secretsmanager.SecretsManager) secret.Plugin {
	return &plugin{
		manager: manager,
	}
}

type plugin struct {
	manager *secretsmanager.SecretsManager
}

func (p *plugin) Find(ctx context.Context, req *secret.Request) (*drone.Secret, error) {
	if req.Path == "" {
		return nil, errors.New("invalid or missing secret path")
	}
	if req.Name == "" {
		return nil, errors.New("invalid or missing secret name")
	}

	// makes an api call to the aws secrets manager and attempts
	// to retrieve the secret at the requested path.
	params, err := p.find(req.Path)
	if err != nil {
		return nil, errors.New("secret not found")
	}
	value := params[req.Name]

	logrus.Infof("Found in %s: %#v", req.Path, params)

	// the user can filter out requets based on event type
	// using the X-Drone-Events secret key. Check for this
	// user-defined filter logic.
	events := extractEvents(params)
	if !match(req.Build.Event, events) {
		return nil, errors.New("access denied: event does not match")
	}

	// the user can filter out requets based on repository
	// using the X-Drone-Repos secret key. Check for this
	// user-defined filter logic.
	repos := extractRepos(params)
	if !match(req.Repo.Slug, repos) {
		return nil, errors.New("access denied: repository does not match")
	}

	// the user can filter out requets based on repository
	// branch using the X-Drone-Branches secret key. Check
	// for this user-defined filter logic.
	branches := extractBranches(params)
	if !match(req.Build.Target, branches) {
		return nil, errors.New("access denied: branch does not match")
	}


	return &drone.Secret{
		Data: value,
		Pull: true, // always true. use X-Drone-Events to prevent pull requests.
		Fork: true, // always true. use X-Drone-Events to prevent pull requests.
	}, nil
}

// helper function returns the secret from the aws secrets manager.
func (p *plugin) find(path string) (map[string]string, error) {
	req := p.manager.GetSecretValueRequest(
		&secretsmanager.GetSecretValueInput{
			SecretId: aws.String(path),
		},
	)
	res, err := req.Send()
	if err != nil {
		return nil, err
	}

	str := aws.StringValue(res.SecretString)
	raw := []byte(str)

	set := map[string]string{}
	err = json.Unmarshal(raw, &set)
	if err != nil {
		wrappedSet := map[string]string{
			"value": string(raw),
		}
		return wrappedSet, nil
	}

	return set, err
}
