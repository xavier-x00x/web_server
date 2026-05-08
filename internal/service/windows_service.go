package service

import (
	"log"

	"github.com/kardianos/service"

	"gopherstack/internal/config"
	"gopherstack/internal/orchestrator"
)

// GopherStackService implements the service.Interface for Windows Service
type GopherStackService struct {
	orch *orchestrator.Orchestrator
	cfg  *config.Config
}

// NewGopherStackService creates a new service wrapper
func NewGopherStackService(cfg *config.Config) *GopherStackService {
	return &GopherStackService{
		cfg: cfg,
	}
}

// Start is called when the service starts
func (s *GopherStackService) Start(svc service.Service) error {
	s.orch = orchestrator.New(s.cfg)

	go func() {
		if err := s.orch.Start(); err != nil {
			log.Fatalf("[service] Failed to start orchestrator: %v", err)
		}
		s.orch.Wait()
	}()

	return nil
}

// Stop is called when the service stops
func (s *GopherStackService) Stop(svc service.Service) error {
	if s.orch != nil {
		s.orch.Stop()
		s.orch.Signal()
	}
	return nil
}

// GetServiceConfig returns the Windows Service configuration
func GetServiceConfig() *service.Config {
	return &service.Config{
		Name:        "GopherStack",
		DisplayName: "GopherStack Enterprise",
		Description: "High-Concurrency PHP Orchestrator for Windows Server - Manages Nginx and PHP worker pools",
		Option: service.KeyValue{
			"StartType": "automatic",
		},
	}
}

// RunAsService runs GopherStack as a Windows Service
func RunAsService(cfg *config.Config) error {
	svcConfig := GetServiceConfig()
	prg := NewGopherStackService(cfg)

	svc, err := service.New(prg, svcConfig)
	if err != nil {
		return err
	}

	return svc.Run()
}

// Install installs GopherStack as a Windows Service
func Install(cfg *config.Config) error {
	svcConfig := GetServiceConfig()
	prg := NewGopherStackService(cfg)

	svc, err := service.New(prg, svcConfig)
	if err != nil {
		return err
	}

	return svc.Install()
}

// Uninstall removes GopherStack from Windows Services
func Uninstall(cfg *config.Config) error {
	svcConfig := GetServiceConfig()
	prg := NewGopherStackService(cfg)

	svc, err := service.New(prg, svcConfig)
	if err != nil {
		return err
	}

	return svc.Uninstall()
}

// ServiceStart starts the Windows Service
func ServiceStart(cfg *config.Config) error {
	svcConfig := GetServiceConfig()
	prg := NewGopherStackService(cfg)

	svc, err := service.New(prg, svcConfig)
	if err != nil {
		return err
	}

	return svc.Start()
}

// ServiceStop stops the Windows Service
func ServiceStop(cfg *config.Config) error {
	svcConfig := GetServiceConfig()
	prg := NewGopherStackService(cfg)

	svc, err := service.New(prg, svcConfig)
	if err != nil {
		return err
	}

	return svc.Stop()
}
