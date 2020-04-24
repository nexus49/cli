package debug

import (
	"bufio"
	"io"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strings"

	"github.com/kyma-project/cli/cmd/kyma/dev"
	"github.com/kyma-project/cli/internal/cli"
	"github.com/kyma-project/cli/internal/kube"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

type command struct {
	opts *Options
	cli.Command
}

//NewCmd creates a new minikube command
func NewCmd(o *Options) *cobra.Command {
	c := command{
		Command: cli.Command{Options: o.Options},
		opts:    o,
	}

	cmd := &cobra.Command{
		Use:   "debug",
		Short: "Debugs the lambda function remotely",
		Long:  `Debugs the lambda function remotely using telepresence`,
		RunE:  func(_ *cobra.Command, _ []string) error { return c.Run() },
	}

	cmd.Flags().StringVarP(&o.WorkingDir, "workdir", "d", "", "Directory where to run the command")

	return cmd
}

func (cmd *command) Run() error {
	if err := cmd.validateFlags(); err != nil {
		return err
	}

	kubeconfigPath := cmd.KubeconfigPath
	if len(cmd.KubeconfigPath) == 0 {
		usr, err := user.Current()
		if err != nil {
			return errors.Wrap(err, "Could not determine home directory")
		}
		kubeconfigPath = filepath.Join(usr.HomeDir, ".kube/config")
	}

	var err error
	if cmd.K8s, err = kube.NewFromConfig("", kubeconfigPath); err != nil {
		return errors.Wrap(err, "Could not initialize the Kubernetes client. Make sure your kubeconfig is valid")
	}

	err = debugFunction(cmd.K8s, cmd.opts.WorkingDir)
	if err != nil {
		return err
	}

	return nil
}

func (c *command) validateFlags() error {
	var errMessage strings.Builder

	if errMessage.Len() != 0 {
		return errors.New(errMessage.String())
	}
	return nil
}

type Config struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	File      string `json:"file"`
	Expose    bool   `json:"expose"`
}

const nodeServiceTemplate = "/Users/i347365/go/src/github.com/nexus49/cli/resources/templates/node-server"

func debugFunction(kube kube.KymaKube, workdir string) error {
	config, currentDir, err := dev.GetConfig(workdir)
	if err != nil {
		return err
	}

	// Create tmp dir for parent app
	// log.Info("Preparing local lambda deployment")
	// outputPath, err := ioutil.TempDir("", "kyma")
	// if err != nil {
	// 	return errors.New("Could not prepare lambda locally")
	// }
	// defer os.RemoveAll(outputPath)

	// sourceFile := filepath.Join(*currentDir, config.File)
	// params := struct {
	// 	Path string
	// }{
	// 	sourceFile,
	// }

	// // iterate through files of template and create files based on parameters
	// var files []string
	// err = filepath.Walk(nodeServiceTemplate, func(path string, info os.FileInfo, err error) error {
	// 	files = append(files, path)
	// 	return nil
	// })
	// if err != nil {
	// 	return errors.Wrap(err, "Could not generate lambda")
	// }
	// for _, file := range files {
	// 	if file != nodeServiceTemplate {
	// 		dev.ProcessTemplateFile(outputPath, nodeServiceTemplate, file, params)
	// 	}
	// }

	indexFilePath := filepath.Join(*currentDir, "local/index.js")
	command := []string{"--namespace", config.Namespace, "--swap-deployment", config.Name, "--expose", "8080", "--run", "nodemon", "--inspect", indexFilePath}
	log.Infof("Running: telepresence %s", strings.Join(command, " "))
	cmd := exec.Command("telepresence", command...)
	cmd.Dir = *currentDir
	stdout, _ := cmd.StdoutPipe()
	stderr, _ := cmd.StderrPipe()
	cmd.Stdin = os.Stdin
	go print(stdout)
	go print(stderr)
	log.Info("Telepresence will ask for your sudo password in order to bind the pod to your local system")
	cmd.Run()

	return nil
}

func print(reader io.ReadCloser) {
	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		log.Info(scanner.Text())
	}
}
