package integration_test

import (
	"context"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

func TestIntegration_NginxContainer(t *testing.T) {
	// Set up environment variables to support rootless Podman out of the box
	os.Setenv("TESTCONTAINERS_RYUK_DISABLED", "true")
	if os.Getenv("DOCKER_HOST") == "" {
		// Set default user-level rootless Podman socket path
		os.Setenv("DOCKER_HOST", "unix:///run/user/1000/podman/podman.sock")
	}

	ctx := context.Background()

	// Spin up a lightweight nginx:alpine container
	req := testcontainers.ContainerRequest{
		Image:        "docker.io/library/nginx:alpine",
		ExposedPorts: []string{"80/tcp"},
		WaitingFor:   wait.ForHTTP("/").WithPort("80/tcp").WithStartupTimeout(30 * time.Second),
	}

	nginxC, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		t.Fatalf("Failed to start Nginx container: %v", err)
	}

	defer func() {
		if err := nginxC.Terminate(ctx); err != nil {
			t.Errorf("Failed to terminate container: %v", err)
		}
	}()

	endpoint, err := nginxC.Endpoint(ctx, "")
	if err != nil {
		t.Fatalf("Failed to get endpoint: %v", err)
	}

	t.Logf("Nginx container successfully running at endpoint: %s", endpoint)

	// Make an actual HTTP request to the running container to verify networking
	resp, err := http.Get("http://" + endpoint)
	if err != nil {
		t.Fatalf("Failed to make GET request to Nginx: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status code 200, got %d", resp.StatusCode)
	}
}
