package dev

import (
	"bufio"
	"encoding/json"
	"fmt"
	"html/template"
	"io/ioutil"
	"os"
	"path/filepath"

	log "github.com/sirupsen/logrus"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

//NewCmd creates a new provision command
func NewCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "dev",
		Short: "Tooling for developing in kyma",
	}
	return cmd
}

func FileExists(path string) bool {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return false
	} else {
		return true
	}
}

type Config struct {
	Name          string `json:"name"`
	Namespace     string `json:"namespace"`
	File          string `json:"file"`
	Expose        bool   `json:"expose"`
	ClusterDomain string `json:"clusterDomain"`
}

func GetConfig(workdir string) (*Config, *string, error) {
	currentDir := workdir
	var err error
	if len(workdir) == 0 {
		currentDir, err = filepath.Abs(filepath.Dir(os.Args[0]))
		if err != nil {
			return nil, nil, err
		}
	}

	path := filepath.Join(currentDir, "kyma.json")
	if !FileExists(path) {
		return nil, nil, errors.New(fmt.Sprintf("kyma.json does not exist at %s.", path))
	}

	b, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, nil, errors.Wrap(err, "Cannot read config file")
	}

	config := &Config{}
	err = json.Unmarshal(b, config)
	if err != nil {
		return nil, nil, errors.Wrap(err, "Cannot marshall config file")
	}
	return config, &currentDir, err
}

func ProcessTemplateFile(outputpath string, templateFolder string, path string, params interface{}) error {
	fi, err := os.Stat(path)
	if err != nil {
		return errors.Wrap(err, "could not find file")
	}

	switch mode := fi.Mode(); {
	case mode.IsDir():
		templatePath := path[len(templateFolder)+1:]
		outputPath := filepath.Join(outputpath, templatePath)

		err = EnsureDir(outputPath)
		if err != nil {
			return err
		}

	case mode.IsRegular():
		templatePath := path[len(templateFolder)+1 : len(path)-5]
		outputPath := filepath.Join(outputpath, templatePath)
		CreateFromTemplate(path, outputPath, params)
	}

	return nil
}

func EnsureDir(path string) error {
	if FileExists(path) {
		log.Infof("[SKIP] %s", path)
	} else {
		// if not create it
		log.Infof("[CREATE] %s", path)
		errDir := os.MkdirAll(path, 0755)
		if errDir != nil {
			return errors.Wrap(errDir, "Could not create directory")
		}
	}

	return nil
}

func CreateFromTemplate(path string, outputPath string, params interface{}) error {
	if !FileExists(outputPath) {
		b, err := ioutil.ReadFile(path)
		if err != nil {
			return errors.Wrap(err, "Cannot read templates")
		}
		stringTemplate := string(b)

		tmpl, err := template.New("out").Parse(stringTemplate)
		if err != nil {
			return errors.Wrap(err, "Cannot parse templates")
		}
		log.Infof("[CREATE] %s", outputPath)
		f, err := os.Create(outputPath)
		defer f.Close()
		if err != nil {
			return errors.Wrap(err, "Failed to create file")
		}
		w := bufio.NewWriter(f)
		err = tmpl.Execute(w, params)
		if err != nil {
			return errors.Wrap(err, "Cannot execute template")
		}
		w.Flush()
	} else {
		log.Infof("[SKIP] %s", outputPath)
	}

	return nil
}
