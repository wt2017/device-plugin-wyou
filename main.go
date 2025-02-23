package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"path"
	"time"

	"google.golang.org/grpc"
	registerapi "k8s.io/kubelet/pkg/apis/pluginregistration/v1"
	pluginapi "k8s.io/kubelet/pkg/apis/deviceplugin/v1beta1"
)

const (
	resourceName = "wyou/ptp"
	socketName   = "wyou-ptp.sock"
	devicePath   = "/dev/ptp0"
)

const (
	rsWatchInterval    = 5 * time.Second
	serverStartTimeout = 5 * time.Second
)

// MyCharDevicePlugin implements the Kubernetes Device Plugin API.
type MyCharDevicePlugin struct {
	devices 			[]*pluginapi.Device
	stop    			chan struct{}
	socketPath         	string
}

// NewMyCharDevicePlugin initializes a new MyCharDevicePlugin.
func NewMyCharDevicePlugin() *MyCharDevicePlugin {
	return &MyCharDevicePlugin{
		devices: []*pluginapi.Device{
			{
				ID:     "dev_ptp_0",
				Health: pluginapi.Healthy,
			},
		},
		stop: make(chan struct{}),
	}
}

// Start starts the gRPC server and registers the device plugin with the kubelet.
func (m *MyCharDevicePlugin) Start() error {
    // Ensure the directory exists
    socketDir := pluginapi.DevicePluginPath
    if err := os.MkdirAll(socketDir, 0755); err != nil {
        return fmt.Errorf("failed to create socket directory: %v", err)
    }

    // Create the socket file
    m.socketPath = path.Join(pluginapi.DevicePluginPath, socketName)
	log.Printf("Socket path: %s", m.socketPath)
    if err := os.Remove(m.socketPath); err != nil && !os.IsNotExist(err) {
        return fmt.Errorf("failed to remove existing socket: %v", err)
    }

    listener, err := net.Listen("unix", m.socketPath)
    if err != nil {
        return fmt.Errorf("failed to listen on socket: %v", err)
    }

    // gRPC server
    server := grpc.NewServer()

    // Register all services
	registerapi.RegisterRegistrationServer(server, m)
	pluginapi.RegisterDevicePluginServer(server, m)

	go func() {
        if err := server.Serve(listener); err != nil {
            log.Fatalf("Failed to start gRPC server: %v", err)
        }
    }()

	// Wait for server to start by launching a blocking connection
	ctx, cancel := context.WithTimeout(context.TODO(), serverStartTimeout)
	defer cancel()
	conn, err := grpc.DialContext(ctx, "unix:"+m.socketPath, grpc.WithInsecure(), grpc.WithBlock())
	if err != nil {
		log.Fatalf("Unable to establish test connection with gRPC server")
		return err
	}
	log.Println("Start device plugin endpoint")
	conn.Close()

    return nil
}

// Stop stops the gRPC server.
func (m *MyCharDevicePlugin) Stop() {
	close(m.stop)
}

func (m *MyCharDevicePlugin) GetInfo(ctx context.Context, rqt *registerapi.InfoRequest) (*registerapi.PluginInfo, error) {
	pluginInfoResponse := &registerapi.PluginInfo{
		Type:              registerapi.DevicePlugin,
		Name:              resourceName,
		Endpoint:          m.socketPath,
		SupportedVersions: []string{"v1alpha1", "v1beta1"},
	}
	return pluginInfoResponse, nil
}

func (m *MyCharDevicePlugin) NotifyRegistrationStatus(ctx context.Context,
	regstat *registerapi.RegistrationStatus,
) (*registerapi.RegistrationStatusResponse, error) {
	if regstat.PluginRegistered {
		log.Println("Plugin: gets registered successfully at Kubelet\n")
	} else {
		log.Fatalf("Plugin: failed to be registered at Kubelet\n")
	}
	return &registerapi.RegistrationStatusResponse{}, nil
}

// ListAndWatch streams the list of devices to the kubelet.
func (m *MyCharDevicePlugin) ListAndWatch(e *pluginapi.Empty, s pluginapi.DevicePlugin_ListAndWatchServer) error {
	for {
		select {
		case <-m.stop:
			return nil
		default:
			s.Send(&pluginapi.ListAndWatchResponse{Devices: m.devices})
			time.Sleep(5 * time.Second)
		}
	}
}

// Allocate allocates devices to containers.
func (m *MyCharDevicePlugin) Allocate(ctx context.Context, req *pluginapi.AllocateRequest) (*pluginapi.AllocateResponse, error) {
	responses := &pluginapi.AllocateResponse{}
	for _, _ = range req.ContainerRequests {
		response := &pluginapi.ContainerAllocateResponse{
			Devices: []*pluginapi.DeviceSpec{
				{
					ContainerPath: devicePath,
					HostPath:      devicePath,
					Permissions:   "rw",
				},
			},
		}
		responses.ContainerResponses = append(responses.ContainerResponses, response)
	}
	return responses, nil
}

// GetDevicePluginOptions returns options for the device plugin.
func (m *MyCharDevicePlugin) GetDevicePluginOptions(ctx context.Context, e *pluginapi.Empty) (*pluginapi.DevicePluginOptions, error) {
	return &pluginapi.DevicePluginOptions{}, nil
}

func (m *MyCharDevicePlugin) GetPreferredAllocation(ctx context.Context, req *pluginapi.PreferredAllocationRequest) (*pluginapi.PreferredAllocationResponse, error) {
    // Implement this method
    return &pluginapi.PreferredAllocationResponse{}, nil
}

// PreStartContainer is called before the container is started.
func (m *MyCharDevicePlugin) PreStartContainer(ctx context.Context, req *pluginapi.PreStartContainerRequest) (*pluginapi.PreStartContainerResponse, error) {
	return &pluginapi.PreStartContainerResponse{}, nil
}

func init() {
    log.SetOutput(os.Stdout)
    log.SetFlags(log.LstdFlags | log.Lshortfile)
}

func main() {
	log.Println("Start plugin")
	plugin := NewMyCharDevicePlugin()
	if err := plugin.Start(); err != nil {
		log.Fatalf("Failed to start device plugin: %v", err)
	}
	defer plugin.Stop()

	// Keep the plugin running
	select {}
}
