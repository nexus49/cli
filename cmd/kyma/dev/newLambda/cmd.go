package newLambda

import (
	"io/ioutil"
	"net/url"
	"os"
	"os/user"
	"path/filepath"
	"strings"

	"github.com/kyma-project/cli/cmd/kyma/dev"
	"github.com/kyma-project/cli/internal/cli"
	"github.com/kyma-project/cli/internal/kube"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v2"
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
		Use:   "new-lambda",
		Short: "New local Lambda Function",
		Long:  `Creates a new local lambda function setup to start development`,
		RunE:  func(_ *cobra.Command, args []string) error { return c.Run(args) },
	}

	cmd.Args = cobra.ExactArgs(1)

	cmd.Flags().StringVarP(&o.Namespace, "namespace", "n", "default", "Namespace to bind")
	cmd.Flags().BoolVar(&o.Expose, "expose", false, "Create the namespace if not existing")
	cmd.Flags().StringVar(&o.ClusterDomain, "cluster-domain", "", "Cluster Domain of your cluster")

	return cmd
}

func (cmd *command) Run(args []string) error {
	if err := cmd.validateFlags(); err != nil {
		return err
	}

	if err := cmd.validateArgs(args); err != nil {
		return err
	}

	name := args[0]

	var err error
	if cmd.K8s, err = kube.NewFromConfig("", cmd.KubeconfigPath); err != nil {
		return errors.Wrap(err, "Could not initialize the Kubernetes client. Make sure your kubeconfig is valid")
	}

	err = createNewTemplate(name, cmd.opts.Namespace, cmd.opts.Expose, cmd.opts.ClusterDomain, cmd.K8s)
	if err != nil {
		return err
	}

	return nil
}

func (c *command) validateArgs(args []string) error {
	var errMessage strings.Builder
	// mandatory flags
	if len(args) != 1 {
		errMessage.WriteString("\nRequired argument `name` has not been set.")
	}

	if errMessage.Len() != 0 {
		return errors.New(errMessage.String())
	}
	return nil
}

func (c *command) validateFlags() error {
	var errMessage strings.Builder
	// mandatory flags
	if c.opts.KubeconfigPath == "" {
		usr, err := user.Current()
		if err != nil {
			return errors.Wrap(err, "Could not determine home directory")
		}
		c.opts.KubeconfigPath = filepath.Join(usr.HomeDir, ".kube/config")
	}

	if c.opts.ClusterDomain == "" {
		clusterDomain, err := getClusterDomainFromKubecofig(c.opts.KubeconfigPath)
		if err != nil {
			return errors.Wrap(err, "Could not determine default value for cluster domain")
		}
		c.opts.ClusterDomain = *clusterDomain
	}

	if errMessage.Len() != 0 {
		return errors.New(errMessage.String())
	}
	return nil
}

const templateFolder = "/Users/i347365/go/src/github.com/nexus49/cli/resources/templates/lambda-javascript"

// const outputFolder = "/Users/i347365/go/src/github.com/nexus49/cli/out"

func createNewTemplate(name string, namespace string, expose bool, clusterDomain string, kube kube.KymaKube) error {
	params := TemplateParameters{
		Name:          name,
		Expose:        expose,
		Namespace:     namespace,
		ClusterDomain: clusterDomain,
	}

	currentDir, err := filepath.Abs(filepath.Dir(os.Args[0]))
	if err != nil {
		return err
	}

	// check if folder exists
	outputPath := filepath.Join(currentDir, name)
	err = dev.EnsureDir(outputPath)
	if err != nil {
		return err
	}

	// iterate through files of template and create files based on parameters
	var files []string
	err = filepath.Walk(templateFolder, func(path string, info os.FileInfo, err error) error {
		files = append(files, path)
		return nil
	})
	if err != nil {
		return errors.Wrap(err, "Could not generate lambda")
	}
	for _, file := range files {
		if file != templateFolder {
			dev.ProcessTemplateFile(outputPath, templateFolder, file, params)
		}
	}
	return nil
}

type TemplateParameters struct {
	Name          string
	Namespace     string
	Expose        bool
	ClusterDomain string
}

func ProcessTemplateFile(outputpath string, path string, params TemplateParameters) error {
	fi, err := os.Stat(path)
	if err != nil {
		return errors.Wrap(err, "could not find file")
	}

	switch mode := fi.Mode(); {
	case mode.IsDir():
		templatePath := path[len(templateFolder)+1:]
		outputPath := filepath.Join(outputpath, templatePath)

		err = dev.EnsureDir(outputPath)
		if err != nil {
			return err
		}

	case mode.IsRegular():
		templatePath := path[len(templateFolder)+1 : len(path)-5]
		outputPath := filepath.Join(outputpath, templatePath)
		dev.CreateFromTemplate(path, outputPath, params)
	}

	return nil
}

func getClusterDomainFromKubecofig(kubeconfigPath string) (*string, error) {
	if !dev.FileExists(kubeconfigPath) {
		return nil, errors.New("Could not find kubeconfig")
	}

	b, err := ioutil.ReadFile(kubeconfigPath)
	if err != nil {
		return nil, errors.Wrap(err, "Cannot read templates")
	}

	m := make(map[string]interface{})
	err = yaml.Unmarshal([]byte(b), &m)
	if err != nil {
		return nil, errors.Wrap(err, "Cannot parse kubeconfig")
	}

	currentContext := m["current-context"].(string)

	// Get Context
	contexts := m["contexts"].([]interface{})

	var currentContextObject map[interface{}]interface{}
	found := false
	for _, ctx := range contexts {
		if ctx.(map[interface{}]interface{})["name"] == currentContext {
			currentContextObject = ctx.(map[interface{}]interface{})
			found = true
			break
		}
	}

	contextContext := currentContextObject["context"].(map[interface{}]interface{})
	clusterRef := contextContext["cluster"].(string)

	clusters := m["clusters"].([]interface{})

	var currentClusterObj map[interface{}]interface{}
	found = false
	for _, cl := range clusters {
		if cl.(map[interface{}]interface{})["name"] == clusterRef {
			currentClusterObj = cl.(map[interface{}]interface{})
			found = true
			break
		}
	}
	if !found {
		return nil, errors.Wrap(err, "Failed to process kubeconfig")
	}

	clusterCluster := currentClusterObj["cluster"].(map[interface{}]interface{})

	apiUrl, err := url.Parse(clusterCluster["server"].(string))

	ind := strings.Index(apiUrl.Host, ".")
	domain := apiUrl.Host[ind+1:]
	return &domain, nil
}
