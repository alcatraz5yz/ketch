package chart

import (
	"errors"
	"fmt"
	"strings"

	v1 "k8s.io/api/core/v1"

	ketchv1 "github.com/theketchio/ketch/internal/api/v1beta1"
)

var (
	ErrPortsNotFound = errors.New("routable process should have at least one container port and one service port")
)

type process struct {
	Name              string             `json:"name"`
	Cmd               []string           `json:"cmd"`
	Units             int                `json:"units"`
	Routable          bool               `json:"routable"`
	ContainerPorts    []v1.ContainerPort `json:"containerPorts"`
	ServicePorts      []v1.ServicePort   `json:"servicePorts"`
	PublicServicePort int32              `json:"publicServicePort,omitempty"`
	Env               []ketchv1.Env      `json:"env"`

	SecurityContext      *v1.SecurityContext      `json:"securityContext,omitempty"`
	ResourceRequirements *v1.ResourceRequirements `json:"resourceRequirements,omitempty"`
	NodeSelectorTerms    []v1.NodeSelectorTerm    `json:"nodeSelectorTerms,omitempty"`
	Volumes              []v1.Volume              `json:"volumes,omitempty"`
	VolumeMounts         []v1.VolumeMount         `json:"volumeMounts,omitempty"`
	ReadinessProbe       *v1.Probe                `json:"readinessProbe,omitempty"`
	LivenessProbe        *v1.Probe                `json:"livenessProbe,omitempty"`
	StartupProbe         *v1.Probe                `json:"startupProbe,omitempty"`
	Lifecycle            *v1.Lifecycle            `json:"lifecycle,omitempty"`
	// ServiceMetadata contains Labels and Annotations to be added to a k8s Service of this process.
	ServiceMetadata extraMetadata `json:"serviceMetadata,omitempty"`
	// DeploymentMetadata contains Labels and Annotations to be added to a k8s Deployment of this process.
	DeploymentMetadata extraMetadata `json:"deploymentMetadata,omitempty"`
	// PodMetadata contains Labels and Annotations to be added to a k8s Pod of this process.
	PodMetadata extraMetadata `json:"podMetadata,omitempty"`
}

type extraMetadata struct {
	Labels      map[string]string `json:"labels,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty"`
}

type processOption func(p *process) error

func withUnits(units *int) processOption {
	return func(p *process) error {
		if units != nil {
			p.Units = *units
		}
		return nil
	}
}

// withEnvs configures env variables of a process.
// Additionally, the process will have port-related envs like "PORT". Check out "portEnvVariables" below.
func withEnvs(envs []ketchv1.Env) processOption {
	return func(p *process) error {
		p.Env = envs
		return nil
	}
}

func withCmd(cmd []string) processOption {
	return func(p *process) error {
		p.Cmd = cmd
		return nil
	}
}

// portConfigurator has methods to work with port related configuration of ketch.yaml.
type portConfigurator interface {
	ContainerPortsForProcess(process string) []v1.ContainerPort
	ServicePortsForProcess(process string) []v1.ServicePort
	Probes() (Probes, error)
}

func withPortsAndProbes(c portConfigurator) processOption {
	return func(p *process) error {
		p.ServicePorts = c.ServicePortsForProcess(p.Name)
		p.ContainerPorts = c.ContainerPortsForProcess(p.Name)
		if len(p.ContainerPorts) == 0 || len(p.ServicePorts) == 0 {
			return nil
		}
		probes, err := c.Probes()
		if err != nil {
			return err
		}
		p.PublicServicePort = p.ServicePorts[0].Port
		p.LivenessProbe = probes.Liveness
		p.ReadinessProbe = probes.Readiness
		p.StartupProbe = probes.StartupProbe
		return nil
	}
}

func withSecurityContext(securityContext *v1.SecurityContext) processOption {
	return func(p *process) error {
		p.SecurityContext = securityContext
		return nil
	}
}

func withLifecycle(lc *v1.Lifecycle) processOption {
	return func(p *process) error {
		p.Lifecycle = lc
		return nil
	}
}

func withResourceRequirements(rr *v1.ResourceRequirements) processOption {
	return func(p *process) error {
		p.ResourceRequirements = rr
		return nil
	}
}

func withVolumes(volumes []v1.Volume) processOption {
	return func(p *process) error {
		p.Volumes = volumes
		return nil
	}
}

func withVolumeMounts(vm []v1.VolumeMount) processOption {
	return func(p *process) error {
		p.VolumeMounts = vm
		return nil
	}
}

// withLabels returns a function that populates Kind labels.
func withLabels(labels []ketchv1.MetadataItem, deploymentVersion ketchv1.DeploymentVersion) processOption {
	return func(p *process) error {
		for _, label := range labels {
			if !canBeApplied(label, p.Name, deploymentVersion) {
				continue
			}
			if err := label.Validate(); err != nil {
				return err
			}
			for k, v := range label.Apply {
				if label.Target.IsDeployment() {
					if p.DeploymentMetadata.Labels == nil {
						p.DeploymentMetadata.Labels = make(map[string]string)
					}
					p.DeploymentMetadata.Labels[k] = v
				} else if label.Target.IsService() {
					if p.ServiceMetadata.Labels == nil {
						p.ServiceMetadata.Labels = make(map[string]string)
					}
					p.ServiceMetadata.Labels[k] = v
				} else if label.Target.IsPod() {
					if p.PodMetadata.Labels == nil {
						p.PodMetadata.Labels = make(map[string]string)
					}
					p.PodMetadata.Labels[k] = v
				}
			}
		}
		return nil
	}
}

// withAnnotations returns a function that populates Kind annotations.
func withAnnotations(annotations []ketchv1.MetadataItem, deploymentVersion ketchv1.DeploymentVersion) processOption {
	return func(p *process) error {
		for _, annotation := range annotations {
			if !canBeApplied(annotation, p.Name, deploymentVersion) {
				continue
			}
			if err := annotation.Validate(); err != nil {
				return err
			}
			for k, v := range annotation.Apply {
				if annotation.Target.IsDeployment() {
					if p.DeploymentMetadata.Annotations == nil {
						p.DeploymentMetadata.Annotations = make(map[string]string)
					}
					p.DeploymentMetadata.Annotations[k] = v
				} else if annotation.Target.IsService() {
					if p.ServiceMetadata.Annotations == nil {
						p.ServiceMetadata.Annotations = make(map[string]string)
					}
					p.ServiceMetadata.Annotations[k] = v
				} else if annotation.Target.IsPod() {
					if p.PodMetadata.Annotations == nil {
						p.PodMetadata.Annotations = make(map[string]string)
					}
					p.PodMetadata.Annotations[k] = v
				}
			}
		}
		return nil
	}
}

// canBeApplied returns true if:
// item.DeploymentVersion is unspecified OR matches deploymentVersion
// item.ProcessName is unspecified OR matches processName
// item.Target.ApiVersion is v1
func canBeApplied(item ketchv1.MetadataItem, processName string, version ketchv1.DeploymentVersion) bool {
	if item.DeploymentVersion > 0 && int(version) != item.DeploymentVersion {
		return false
	}
	if item.ProcessName != "" && processName != item.ProcessName {
		return false
	}
	return true
}

func newProcess(name string, isRoutable bool, opts ...processOption) (*process, error) {
	process := &process{
		Name:     name,
		Units:    ketchv1.DefaultNumberOfUnits,
		Routable: isRoutable,
	}

	for _, opt := range opts {
		if err := opt(process); err != nil {
			return nil, err
		}
	}

	process.Env = append(process.Env, process.portEnvVariables()...)
	if !process.Routable {
		return process, nil
	}
	// only routable process must have configured ports.
	if !process.hasOpenPort() {
		return nil, ErrPortsNotFound
	}
	return process, nil
}

func (p process) hasOpenPort() bool {
	return len(p.ContainerPorts) > 0 && len(p.ServicePorts) > 0
}

func (p process) portEnvVariables() []ketchv1.Env {
	if len(p.ContainerPorts) == 0 {
		return nil
	}
	var envs []ketchv1.Env
	if len(p.ContainerPorts) == 1 {
		portValue := fmt.Sprintf("%d", p.ContainerPorts[0].ContainerPort)
		envs = append(envs, ketchv1.Env{Name: "port", Value: portValue})
		envs = append(envs, ketchv1.Env{Name: "PORT", Value: portValue})
	}
	ports := make([]string, 0, len(p.ContainerPorts))
	for _, port := range p.ContainerPorts {
		ports = append(ports, fmt.Sprintf("%d", port.ContainerPort))
	}
	envs = append(envs, ketchv1.Env{Name: fmt.Sprintf("PORT_%s", p.Name), Value: strings.Join(ports, ",")})
	return envs
}
