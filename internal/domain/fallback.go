package domain

import (
	"sort"
	"strings"
)

type FallbackConfig struct {
	Threshold int
	Models    map[string]string
	Efforts   map[string]string
}

type StartRequest struct {
	Harness Harness
	Model   string
	Effort  string
}

type FallbackOffer struct {
	Advice  SwitchAdvice
	Request StartRequest
}

var codexEfforts = map[string]string{"low": "low", "medium": "medium", "high": "high", "xhigh": "high", "max": "high"}

func OfferFallback(quotas []Quota, cfg FallbackConfig, req StartRequest) (FallbackOffer, bool) {
	if req.Harness != HarnessClaude {
		return FallbackOffer{}, false
	}
	threshold := cfg.Threshold
	if threshold <= 0 {
		threshold = WarnQuotaLeft
	}
	advice, ok := AdviseAt(quotas, HarnessClaude, threshold)
	if !ok || advice.OtherShortest == nil || advice.OtherShortest.LeftPercent < threshold {
		return FallbackOffer{}, false
	}
	return FallbackOffer{
		Advice: advice,
		Request: StartRequest{
			Harness: HarnessCodex,
			Model:   mapModel(cfg.Models, req.Model),
			Effort:  mapEffort(cfg.Efforts, req.Effort),
		},
	}, true
}

func OfferFallbacks(quotas []Quota, cfg FallbackConfig, queue []StartRequest) map[int]FallbackOffer {
	out := map[int]FallbackOffer{}
	for i, req := range queue {
		if offer, ok := OfferFallback(quotas, cfg, req); ok {
			out[i] = offer
		}
	}
	return out
}

func mapModel(models map[string]string, model string) string {
	model = strings.ToLower(strings.TrimSpace(model))
	if model == "" {
		return models["default"]
	}
	keys := make([]string, 0, len(models))
	for k := range models {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if len(keys[i]) != len(keys[j]) {
			return len(keys[i]) > len(keys[j])
		}
		return keys[i] < keys[j]
	})
	for _, k := range keys {
		if k != "" && k != "default" && strings.Contains(model, strings.ToLower(k)) {
			return models[k]
		}
	}
	return ""
}

func mapEffort(efforts map[string]string, effort string) string {
	effort = strings.ToLower(strings.TrimSpace(effort))
	if v, ok := efforts[effort]; ok {
		return v
	}
	return codexEfforts[effort]
}
