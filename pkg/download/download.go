// @license
// Copyright 2021 Dynatrace LLC
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

package download

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/dynatrace-oss/dynatrace-monitoring-as-code/pkg/api"
	"github.com/dynatrace-oss/dynatrace-monitoring-as-code/pkg/download/jsoncreator"
	"github.com/dynatrace-oss/dynatrace-monitoring-as-code/pkg/download/yamlcreator"
	"github.com/dynatrace-oss/dynatrace-monitoring-as-code/pkg/environment"
	"github.com/dynatrace-oss/dynatrace-monitoring-as-code/pkg/rest"
	"github.com/dynatrace-oss/dynatrace-monitoring-as-code/pkg/util/log"
	"github.com/spf13/afero"
)

var cont = 0

//GetConfigsFilterByEnvironment filters the enviroments list based on specificEnvironment flag value
func GetConfigsFilterByEnvironment(workingDir string, fs afero.Fs, environmentsFile string,
	specificEnvironment string, downloadSpecificAPI string) error {
	environments, errors := environment.LoadEnvironmentList(specificEnvironment, environmentsFile, fs)
	if len(errors) > 0 {
		for _, err := range errors {
			log.Error("Error while getting enviroments ", err)
		}
		return fmt.Errorf("There were some errors while getting environment files")
	}
	return getConfigs(fs, workingDir, environments, downloadSpecificAPI)

}

//getConfigs Entry point that retrieves the specified configurations from a Dynatrace tenant
func getConfigs(fs afero.Fs, workingDir string, environments map[string]environment.Environment, downloadSpecificAPI string) error {
	list, err := getAPIList(downloadSpecificAPI)
	if err != nil {
		return err
	}
	isError := false
	for _, environment := range environments {
		//download configs for each environment
		err := downloadConfigFromEnvironment(fs, environment, workingDir, list)
		if err != nil {
			log.Error("error while downloading configs for environment %v %v", environment.GetId())
			isError = true
		}
	}
	if isError {
		return fmt.Errorf("There were some errors while downloading the environment configs, please check the logs")
	}
	return nil

}

//returns the list of API filter if the download specific flag is used, otherwise returns all the API's
func getAPIList(downloadSpecificAPI string) (filterAPIList map[string]api.Api, err error) {
	availableApis := api.NewApis()
	noFilterAPIListProvided := strings.TrimSpace(downloadSpecificAPI) == ""

	if noFilterAPIListProvided {
		return availableApis, nil
	}
	requestedApis := strings.Split(downloadSpecificAPI, ",")
	isErr := false
	filterAPIList = make(map[string]api.Api)
	for _, id := range requestedApis {
		cleanAPI := strings.TrimSpace(id)
		isAPI := api.IsApi(cleanAPI)
		if !isAPI {
			log.Error("Value %s is not a valid API name", cleanAPI)
			isErr = true
		} else {
			filterAPI := availableApis[cleanAPI]
			filterAPIList[cleanAPI] = filterAPI
		}
	}
	if isErr {
		return nil, fmt.Errorf("There were some errors in the API list provided")
	}

	return filterAPIList, nil
}

//creates the project and downloads the configs
func downloadConfigFromEnvironment(fs afero.Fs, environment environment.Environment, basepath string, listApis map[string]api.Api) (err error) {
	projectName := environment.GetId()
	path := filepath.Join(basepath, projectName)

	log.Info("Creating base project name %s", projectName)
	err = fs.MkdirAll(path, 0777)
	if err != nil {
		log.Error("error creating folder for enviroment %v %v", projectName, err)
		return err
	}
	token, err := environment.GetToken()
	if err != nil {
		log.Error("error retrieving token for enviroment %v %v", projectName, err)
		return err
	}
	client, err := rest.NewDynatraceClient(environment.GetEnvironmentUrl(), token)
	if err != nil {
		log.Error("error creating dynatrace client for enviroment %v %v", projectName, err)
		return err
	}
	for _, api := range listApis {
		log.Info(" --- GETTING CONFIGS for %s", api.GetId())
		jcreator := jsoncreator.NewJSONCreator()
		ycreator := yamlcreator.NewYamlConfig()
		errorAPI := createConfigsFromAPI(fs, api, token, path, client, jcreator, ycreator)
		if errorAPI != nil {
			log.Error("error getting configs from API %v %v", api.GetId())
		}
	}
	log.Info("END downloading info %s", projectName)
	return nil
}

func createConfigsFromAPI(fs afero.Fs, api api.Api, token string, fullpath string, client rest.DynatraceClient,
	jcreator jsoncreator.JSONCreator, ycreator yamlcreator.YamlCreator) (err error) {
	//retrieves all objects for the specific api
	values, err := client.List(api)
	if err != nil {
		log.Error("error getting client list from api %v %v", api.GetId(), err)
		return err
	}
	if len(values) == 0 {
		log.Info("No elements for API %s", api.GetId())
		return nil
	}
	subPath := filepath.Join(fullpath, api.GetId())
	err = fs.MkdirAll(subPath, 0777)
	if err != nil {
		log.Error("error creating folder for api %v %v", api.GetId(), err)
		return err
	}
	for _, val := range values {
		log.Debug("getting detail %s", val)
		cont++
		log.Debug("REQUEST counter %v", cont)
		name, cleanName, filter, err := jcreator.CreateJSONConfig(fs, client, api, val, subPath)
		if err != nil {
			log.Error("error creating config api json file: %v", err)
			continue
		}
		if filter {
			continue
		}
		ycreator.AddConfig(cleanName, name)
	}

	err = ycreator.CreateYamlFile(fs, subPath, api.GetId())
	if err != nil {
		log.Error("error creating config api yaml file: %v", err)
		return err
	}
	return nil
}
