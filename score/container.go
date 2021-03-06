package score

import (
	"strings"

	"github.com/zegl/kube-score/scorecard"

	corev1 "k8s.io/api/core/v1"
)

func scoreContainerLimits(podTemplate corev1.PodTemplateSpec) (score scorecard.TestScore) {
	score.Name = "Container Resources"

	pod := podTemplate.Spec

	allContainers := pod.InitContainers
	allContainers = append(allContainers, pod.Containers...)

	hasMissingLimit := false
	hasMissingRequest := false

	for _, container := range allContainers {
		if container.Resources.Limits.Cpu().IsZero() {
			score.AddComment(container.Name, "CPU limit is not set", "Resource limits are recommended to avoid resource DDOS. Set resources.limits.cpu")
			hasMissingLimit = true
		}
		if container.Resources.Limits.Memory().IsZero() {
			score.AddComment(container.Name, "Memory limit is not set", "Resource limits are recommended to avoid resource DDOS. Set resources.limits.memory")
			hasMissingLimit = true
		}
		if container.Resources.Requests.Cpu().IsZero() {
			score.AddComment(container.Name, "CPU request is not set", "Resource requests are recommended to make sure that the application can start and run without crashing. Set resources.requests.cpu")
			hasMissingRequest = true
		}
		if container.Resources.Requests.Memory().IsZero() {
			score.AddComment(container.Name, "Memory request is not set", "Resource requests are recommended to make sure that the application can start and run without crashing. Set resources.requests.memory")
			hasMissingRequest = true
		}
	}

	if len(allContainers) == 0 {
		score.Grade = 0
		score.AddComment("", "No containers defined", "")
	} else if hasMissingLimit {
		score.Grade = 0
	} else if hasMissingRequest {
		score.Grade = 5
	} else {
		score.Grade = 10
	}

	return
}

func scoreContainerImageTag(podTemplate corev1.PodTemplateSpec) (score scorecard.TestScore) {
	score.Name = "Container Image Tag"

	pod := podTemplate.Spec

	allContainers := pod.InitContainers
	allContainers = append(allContainers, pod.Containers...)

	hasTagLatest := false

	for _, container := range allContainers {
		imageParts := strings.Split(container.Image, ":")
		imageVersion := imageParts[len(imageParts)-1]

		if imageVersion == "latest" {
			score.AddComment(container.Name, "Image with latest tag", "Using a fixed tag is recommended to avoid accidental upgrades")
			hasTagLatest = true
		}
	}

	if hasTagLatest {
		score.Grade = 0
	} else {
		score.Grade = 10
	}

	return
}

func scoreContainerImagePullPolicy(podTemplate corev1.PodTemplateSpec) (score scorecard.TestScore) {
	score.Name = "Container Image Pull Policy"

	pod := podTemplate.Spec

	allContainers := pod.InitContainers
	allContainers = append(allContainers, pod.Containers...)

	hasNonAlways := false

	for _, container := range allContainers {
		if container.ImagePullPolicy != corev1.PullAlways {
			score.AddComment(container.Name, "ImagePullPolicy is not set to PullAlways", "It's recommended to always set the ImagePullPolicy to PullAlways, to make sure that the imagePullSecrets are always correct, and to always get the image you want.")
			hasNonAlways = true
		}
	}

	if hasNonAlways {
		score.Grade = 0
	} else {
		score.Grade = 10
	}

	return
}

func scoreContainerProbes(allServices []corev1.Service) func(corev1.PodTemplateSpec) scorecard.TestScore {
	return func(podTemplate corev1.PodTemplateSpec) (score scorecard.TestScore) {
		score.Name = "Pod Probes"

		allContainers := podTemplate.Spec.InitContainers
		allContainers = append(allContainers, podTemplate.Spec.Containers...)

		hasReadinessProbe := false
		hasLivenessProbe := false
		probesAreIdentical := false
		isTargetedByService := false

		for _, service := range allServices {
			for selectorKey, selectorVal := range service.Spec.Selector {
				if podLabelVal, ok := podTemplate.Labels[selectorKey]; ok && podLabelVal == selectorVal {
					isTargetedByService = true
				}
			}
		}

		for _, container := range allContainers {
			if container.ReadinessProbe != nil {
				hasReadinessProbe = true
			} else {
				if isTargetedByService {
					score.AddComment(container.Name, "Container is missing a readinessProbe", "Without a readinessProbe Services will start sending traffic to this pod before it's ready")
				}
			}

			if container.LivenessProbe != nil {
				hasLivenessProbe = true
			} else {
				score.AddComment(container.Name, "Container is missing a livenessProbe", "Without a livenessProbe kubelet can not restart the Pod if it has crashed")
			}

			if container.ReadinessProbe != nil && container.LivenessProbe != nil {

				r := container.ReadinessProbe
				l := container.LivenessProbe

				if r.HTTPGet != nil && l.HTTPGet != nil {
					if r.HTTPGet.Path == l.HTTPGet.Path &&
						r.HTTPGet.Port.IntValue() == l.HTTPGet.Port.IntValue() {
						probesAreIdentical = true
						score.AddComment(container.Name, "Container has the same readiness and liveness probe", "It's recommended to have different probes for the two different purposes.")
					}
				}

				if r.TCPSocket != nil && l.TCPSocket != nil {
					if r.TCPSocket.Port == l.TCPSocket.Port {
						probesAreIdentical = true
						score.AddComment(container.Name, "Container has the same readiness and liveness probe", "It's recommended to have different probes for the two different purposes.")
					}
				}

				if r.Exec != nil && l.Exec != nil {
					if len(r.Exec.Command) == len(l.Exec.Command) {
						hasDifferent := false
						for i, v := range r.Exec.Command {
							if l.Exec.Command[i] != v {
								hasDifferent = true
								break
							}
						}

						if !hasDifferent {
							probesAreIdentical = true
							score.AddComment(container.Name, "Container has the same readiness and liveness probe", "It's recommended to have different probes for the two different purposes.")
						}
					}
				}

			}
		}

		if hasLivenessProbe && (hasReadinessProbe || !isTargetedByService) {
			if !probesAreIdentical {
				score.Grade = 10
			} else {
				score.Grade = 7
			}
		} else if !hasReadinessProbe && !hasLivenessProbe {
			score.Grade = 0
		} else if isTargetedByService && !hasReadinessProbe {
			score.Grade = 0
		} else if !hasLivenessProbe {
			score.Grade = 5
		} else {
			score.Grade = 0
		}

		return score
	}
}

func scoreContainerSecurityContext(podTemplate corev1.PodTemplateSpec) (score scorecard.TestScore) {
	score.Name = "Container Security Context"

	allContainers := podTemplate.Spec.InitContainers
	allContainers = append(allContainers, podTemplate.Spec.Containers...)

	hasPrivileged := false
	hasWritableRootFS := false
	hasLowUserID := false
	hasLowGroupID := false

	for _, container := range allContainers {

		if container.SecurityContext == nil {
			continue
		}

		sec := container.SecurityContext

		if sec.Privileged != nil && *sec.Privileged {
			hasPrivileged = true
			score.AddComment(container.Name, "The container is privileged", "Set securityContext.Privileged to false")
		}

		if sec.ReadOnlyRootFilesystem != nil && *sec.ReadOnlyRootFilesystem == false {
			hasWritableRootFS = true
			score.AddComment(container.Name, "The pod has a container with a writable root filesystem", "Set securityContext.ReadOnlyFileSystem to true")
		}

		if sec.RunAsUser != nil && *sec.RunAsUser < 10000 {
			hasLowUserID = true
			score.AddComment(container.Name, "The container is running with a low user ID", "A userid above 10 000 is recommended to avoid conflicts with the host. Set securityContext.RunAsUser to a value > 10000")
		}

		if sec.RunAsGroup != nil && *sec.RunAsGroup < 10000 {
			hasLowGroupID = true
			score.AddComment(container.Name, "The container running with a low group ID", "A groupid above 10 000 is recommended to avoid conflicts with the host. Set securityContext.RunAsGroup to a value > 10000")
		}
	}

	if hasPrivileged || hasWritableRootFS || hasLowUserID || hasLowGroupID {
		score.Grade = 0
	} else {
		score.Grade = 10
	}

	return
}
