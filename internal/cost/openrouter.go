package cost

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	DefaultOpenRouterURL = "https://openrouter.ai/api/v1/models"
	fetchTimeout         = 15 * time.Second
)

// PricingSource provides model pricing lookup.
type PricingSource interface {
	Lookup(model string) (ModelPricing, bool)
}

// openRouterModel matches the relevant fields from the OpenRouter /api/v1/models response.
type openRouterModel struct {
	ID      string `json:"id"`
	Pricing struct {
		Prompt     string `json:"prompt"`
		Completion string `json:"completion"`
	} `json:"pricing"`
}

type openRouterResponse struct {
	Data []openRouterModel `json:"data"`
}

// OpenRouterSource fetches and caches pricing data from the OpenRouter API.
type OpenRouterSource struct {
	mu       sync.RWMutex
	prices   map[string]ModelPricing
	url      string
	client   *http.Client
	fetchMu  sync.Mutex
	fetched  bool
	fetchErr error
}

// NewOpenRouterSource creates a new pricing source backed by the OpenRouter API.
// The URL parameter is optional; if empty, DefaultOpenRouterURL is used.
// The httpClient parameter is optional; if nil, a default client with a 15-second timeout is used.
func NewOpenRouterSource(url string, httpClient *http.Client) *OpenRouterSource {
	if strings.TrimSpace(url) == "" {
		url = DefaultOpenRouterURL
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: fetchTimeout}
	}
	return &OpenRouterSource{
		prices: make(map[string]ModelPricing),
		url:    url,
		client: httpClient,
	}
}

// Fetch retrieves pricing data from the OpenRouter API and caches it.
// It should be called once at startup. Returns an error if the fetch fails.
func (s *OpenRouterSource) Fetch(ctx context.Context) error {
	prices, err := s.fetchPrices(ctx)
	s.fetchMu.Lock()
	defer s.fetchMu.Unlock()
	s.fetched = true
	s.fetchErr = err
	if err != nil {
		return err
	}
	s.mu.Lock()
	s.prices = prices
	s.mu.Unlock()
	return nil
}

func (s *OpenRouterSource) fetchPrices(ctx context.Context) (map[string]ModelPricing, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.url, nil)
	if err != nil {
		return nil, fmt.Errorf("build openrouter request: %w", err)
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch openrouter models: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("openrouter returned status %d", resp.StatusCode)
	}
	var result openRouterResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode openrouter response: %w", err)
	}
	prices := make(map[string]ModelPricing, len(result.Data))
	for _, m := range result.Data {
		inputPrice, err1 := strconv.ParseFloat(m.Pricing.Prompt, 64)
		outputPrice, err2 := strconv.ParseFloat(m.Pricing.Completion, 64)
		if err1 != nil || err2 != nil {
			continue
		}
		if inputPrice == 0 && outputPrice == 0 {
			continue
		}
		prices[m.ID] = ModelPricing{
			InputPerToken:  inputPrice,
			OutputPerToken: outputPrice,
		}
	}
	return prices, nil
}

// Lookup finds pricing for a model. It tries the exact model name first,
// then falls back to "anthropic/<model>" (the OpenRouter convention for
// Anthropic models).
func (s *OpenRouterSource) Lookup(model string) (ModelPricing, bool) {
	s.fetchMu.Lock()
	if !s.fetched {
		ctx, cancel := context.WithTimeout(context.Background(), fetchTimeout)
		prices, err := s.fetchPrices(ctx)
		cancel()
		s.fetched = true
		s.fetchErr = err
		if err == nil {
			s.mu.Lock()
			s.prices = prices
			s.mu.Unlock()
		}
	}
	s.fetchMu.Unlock()
	s.mu.RLock()
	defer s.mu.RUnlock()
	if p, ok := s.prices[model]; ok {
		return p, true
	}
	if p, ok := s.prices["anthropic/"+model]; ok {
		return p, true
	}
	return ModelPricing{}, false
}

// ModelCount returns how many models have pricing loaded.
func (s *OpenRouterSource) ModelCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.prices)
}
