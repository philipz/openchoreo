// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/openchoreo/openchoreo/test/utils"
)

const (
	observabilityNamespace = "openchoreo-observability-plane"
	helmReleaseName        = "openchoreo-observability-plane"
)

var _ = Describe("ClickStack Observability Plane", Ordered, func() {
	skipMsg := "Set E2E_CLICKSTACK=true with a reachable cluster to run ClickStack e2e validations"
	var ingestedLogMessage string

	BeforeAll(func() {
		if os.Getenv("E2E_CLICKSTACK") != "true" {
			Skip(skipMsg)
		}
		By("installing the ClickStack observability plane via Helm (minimal footprint)")
		cmd := exec.Command("helm", "upgrade", "--install", helmReleaseName,
			"./install/helm/openchoreo-observability-plane",
			"--namespace", observabilityNamespace,
			"--create-namespace",
			"--set", "global.installationMode=minimal",
			"--set", "collectors.enabled=false",
			"--set", "openSearch.enabled=false",
			"--set", "openSearchCluster.enabled=false",
			"--set", "observer.telemetry.backend=clickstack",
			"--set", "observer.telemetry.dualRead=false",
			"--set", "observer.openSearch.address=http://127.0.0.1:9200",
			"--set", "observer.openSearch.username=clickstack",
			"--set", "observer.openSearch.password=clickstack",
			"--set", "hyperdx.env.HYPERDX_API_KEY=openchoreo-dev",
			"--set", "gateway.config.exporters.clickhouse.endpoint=tcp://clickstack:9000",
			"--set", "hyperdx.resources.requests.cpu=200m",
			"--set", "hyperdx.resources.requests.memory=512Mi",
			"--set", "hyperdx.resources.limits.cpu=400m",
			"--set", "hyperdx.resources.limits.memory=1Gi",
			"--set", "clickstack.storage.size=10Gi",
			"--set", "clickstack.resources.requests.cpu=200m",
			"--set", "clickstack.resources.requests.memory=1Gi",
			"--set", "clickstack.resources.limits.cpu=1000m",
			"--set", "clickstack.resources.limits.memory=2Gi",
			"--set", "gateway.resources.requests.cpu=100m",
			"--set", "gateway.resources.requests.memory=128Mi",
			"--set", "gateway.resources.limits.cpu=200m",
			"--set", "gateway.resources.limits.memory=256Mi",
			"--set", "observer.resources.requests.cpu=50m",
			"--set", "observer.resources.requests.memory=128Mi",
			"--set", "observer.resources.limits.cpu=200m",
			"--set", "observer.resources.limits.memory=256Mi",
			"--set", "hyperdx.resources.requests.cpu=100m",
			"--set", "hyperdx.resources.requests.memory=128Mi",
			"--set", "hyperdx.resources.limits.cpu=200m",
			"--set", "hyperdx.resources.limits.memory=256Mi",
			"--wait",
			"--timeout", "15m",
		)
		_, err := utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "failed to install ClickStack chart")

		By("deploying observability collectors via make target")
		cmd = exec.Command("make", "deploy-observability")
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "make deploy-observability should succeed")
	})

	AfterAll(func() {
		if os.Getenv("E2E_CLICKSTACK") != "true" {
			return
		}
		By("uninstalling the ClickStack Helm release")
		cmd := exec.Command("helm", "uninstall", helmReleaseName, "--namespace", observabilityNamespace, "--wait")
		_, _ = utils.Run(cmd)
		By("cleaning up observability namespace")
		cmd = exec.Command("kubectl", "delete", "ns", observabilityNamespace, "--ignore-not-found=true")
		_, _ = utils.Run(cmd)
	})

	It("deploys core ClickStack components and collectors", func() {
		if os.Getenv("E2E_CLICKSTACK") != "true" {
			Skip(skipMsg)
		}

		waitForStatefulSetReady("clickstack", 1)
		waitForStatefulSetReady("hyperdx-mongodb", 1)
		waitForDeploymentReady("otlp-gateway", 1)
		waitForDeploymentReady("observer", 1)
		waitForDeploymentReady("hyperdx", 1)
		waitForDaemonSetReady("otel-collector")
	})

	It("exposes Grafana dashboards ConfigMaps for ClickStack", func() {
		if os.Getenv("E2E_CLICKSTACK") != "true" {
			Skip(skipMsg)
		}
		cmd := exec.Command("kubectl", "get", "configmap",
			"-n", observabilityNamespace,
			"-l", "grafana_dashboard=1")
		output, err := utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "grafana dashboard configmaps should exist")
		Expect(output).To(ContainSubstring("clickstack-overview"), "clickstack overview dashboard missing")
	})

	It("ingests logs via the OTLP gateway and queries them from ClickStack", func() {
		if os.Getenv("E2E_CLICKSTACK") != "true" {
			Skip(skipMsg)
		}

		msg := fmt.Sprintf("clickstack-e2e-log-%d", time.Now().UnixNano())
		Expect(sendOTLPLog(observabilityNamespace, msg)).To(Succeed(), "failed to push log via OTLP HTTP")

		Eventually(func(g Gomega) {
			count, err := clickstackLogCount(msg)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(count).To(BeNumerically(">", 0), "log should be queryable from ClickStack")
		}, "2m", "5s").Should(Succeed())
		ingestedLogMessage = msg
	})

	It("recovers from a ClickStack pod restart without losing data", func() {
		if os.Getenv("E2E_CLICKSTACK") != "true" {
			Skip(skipMsg)
		}
		Expect(ingestedLogMessage).NotTo(BeEmpty(), "ingestion test must run before failover test")

		cmd := exec.Command("kubectl", "delete", "pod", "clickstack-0",
			"-n", observabilityNamespace, "--wait=false")
		_, err := utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "failed to delete clickstack pod")

		Eventually(func(g Gomega) {
			status, err := kubectlOutput("get", "pod", "clickstack-0",
				"-n", observabilityNamespace,
				"-o", "jsonpath={.status.phase}")
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(strings.TrimSpace(status)).To(Equal("Running"))

			ready, err := kubectlOutput("get", "pod", "clickstack-0",
				"-n", observabilityNamespace,
				"-o", "jsonpath={.status.containerStatuses[0].ready}")
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(strings.TrimSpace(ready)).To(Equal("true"))
		}, "3m", "10s").Should(Succeed())

		Eventually(func(g Gomega) {
			count, err := clickstackLogCount(ingestedLogMessage)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(count).To(BeNumerically(">", 0), "log should persist after failover")
		}, "2m", "5s").Should(Succeed())
	})
})

func waitForStatefulSetReady(name string, replicas int) {
	Eventually(func(g Gomega) {
		readyStr, err := kubectlOutput("get", "statefulset", name,
			"-n", observabilityNamespace,
			"-o", "jsonpath={.status.readyReplicas}")
		g.Expect(err).NotTo(HaveOccurred())
		ready := parseCount(readyStr)
		g.Expect(ready).To(Equal(replicas), "statefulset %s should have %d ready replicas", name, replicas)
	}, "10m", "10s").Should(Succeed())
}

func waitForDeploymentReady(name string, replicas int) {
	Eventually(func(g Gomega) {
		readyStr, err := kubectlOutput("get", "deployment", name,
			"-n", observabilityNamespace,
			"-o", "jsonpath={.status.readyReplicas}")
		g.Expect(err).NotTo(HaveOccurred())
		ready := parseCount(readyStr)
		g.Expect(ready).To(Equal(replicas), "deployment %s should have %d ready replicas", name, replicas)
	}, "10m", "10s").Should(Succeed())
}

func waitForDaemonSetReady(name string) {
	Eventually(func(g Gomega) {
		status, err := kubectlOutput("get", "daemonset", name,
			"-n", observabilityNamespace,
			"-o", "jsonpath={.status.numberReady}/{.status.desiredNumberScheduled}")
		g.Expect(err).NotTo(HaveOccurred())
		parts := strings.Split(strings.TrimSpace(status), "/")
		g.Expect(parts).To(HaveLen(2))
		g.Expect(parts[0]).To(Equal(parts[1]), "daemonset %s should have all pods ready", name)
	}, "5m", "10s").Should(Succeed())
}

func kubectlOutput(args ...string) (string, error) {
	cmd := exec.Command("kubectl", args...)
	return utils.Run(cmd)
}

func parseCount(value string) int {
	v := strings.TrimSpace(value)
	if v == "" {
		return 0
	}
	count, err := strconv.Atoi(v)
	if err != nil {
		return 0
	}
	return count
}

func sendOTLPLog(namespace, message string) error {
	payload, err := buildOTLPLogPayload(message)
	if err != nil {
		return err
	}

	const localPort = 14318
	pf, err := startPortForward(namespace, "svc/otlp-gateway", localPort, 4318)
	if err != nil {
		return err
	}
	defer pf.Close()

	req, err := http.NewRequestWithContext(context.Background(),
		http.MethodPost,
		fmt.Sprintf("http://127.0.0.1:%d/v1/logs", localPort),
		bytes.NewReader(payload),
	)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("unexpected status from OTLP gateway: %s", resp.Status)
	}

	return nil
}

func buildOTLPLogPayload(message string) ([]byte, error) {
	now := time.Now().UTC()
	payload := map[string]interface{}{
		"resourceLogs": []map[string]interface{}{
			{
				"resource": map[string]interface{}{
					"attributes": []map[string]interface{}{
						{"key": "service.name", "value": map[string]interface{}{"stringValue": "clickstack-e2e"}},
						{"key": "organization.id", "value": map[string]interface{}{"stringValue": "org-e2e"}},
						{"key": "project.id", "value": map[string]interface{}{"stringValue": "proj-e2e"}},
						{"key": "component.id", "value": map[string]interface{}{"stringValue": "comp-e2e"}},
						{"key": "environment.id", "value": map[string]interface{}{"stringValue": "env-e2e"}},
					},
				},
				"scopeLogs": []map[string]interface{}{
					{
						"scope": map[string]interface{}{
							"name": "clickstack-e2e-suite",
						},
						"logRecords": []map[string]interface{}{
							{
								"timeUnixNano": fmt.Sprintf("%d", now.UnixNano()),
								"severityText": "INFO",
								"body": map[string]interface{}{
									"stringValue": message,
								},
								"attributes": []map[string]interface{}{
									{"key": "namespace", "value": map[string]interface{}{"stringValue": observabilityNamespace}},
									{"key": "pod_id", "value": map[string]interface{}{"stringValue": "e2e-pod"}},
									{"key": "log_type", "value": map[string]interface{}{"stringValue": "application"}},
								},
							},
						},
					},
				},
			},
		},
	}

	return json.Marshal(payload)
}

type portForward struct {
	cancel context.CancelFunc
	done   chan struct{}
}

func startPortForward(namespace, resource string, localPort, remotePort int) (*portForward, error) {
	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, "kubectl", "-n", namespace, "port-forward", resource,
		fmt.Sprintf("%d:%d", localPort, remotePort))
	if dir, err := utils.GetProjectDir(); err == nil {
		cmd.Dir = dir
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Start(); err != nil {
		cancel()
		return nil, err
	}

	done := make(chan struct{})
	go func() {
		_ = cmd.Wait()
		close(done)
	}()

	addr := fmt.Sprintf("127.0.0.1:%d", localPort)
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", addr, time.Second)
		if err == nil {
			_ = conn.Close()
			return &portForward{cancel: cancel, done: done}, nil
		}
		time.Sleep(200 * time.Millisecond)
	}

	cancel()
	<-done
	return nil, fmt.Errorf("port-forward for %s failed: %s %s", resource, stdout.String(), stderr.String())
}

func (pf *portForward) Close() {
	pf.cancel()
	<-pf.done
}

func clickstackLogCount(message string) (int, error) {
	query := "SELECT count() FROM telemetry.logs_mv WHERE log = {message:String}"
	output, err := kubectlOutput("exec", "-n", observabilityNamespace, "clickstack-0", "--",
		"clickhouse-client",
		"--query", query,
		"--param_message", message,
	)
	if err != nil {
		return 0, err
	}
	return parseCount(output), nil
}
