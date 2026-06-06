package memory

import ()

// FeedObservation is the single entry point for external data sources (QQ, browser,
// screen) to feed observations into the memory self-learning pipeline.
func (p *MemoryPlugin) FeedObservation(obs Observation) {
	if p.proactive != nil {
		p.proactive.Ingest(obs)
	}
	if p.careEngine != nil {
		p.careEngine.UpdateState(obs)
	}
}

// DetectKnowledgeGaps scans for missing or stale information and generates
// up to 3 natural questions per day.

func (p *MemoryPlugin) summarizeAndAssignTopic(epID int64) {
	defer recoverGuard("summarizeAndAssignTopic")
	if p.store == nil || p.episodeStore == nil || p.topicStoreInst == nil {
		return
	}
	if err := p.episodeStore.SummarizeEpisode(epID, p.rawLLM); err != nil {
		return
	}
	episodes := p.episodeStore.ListActive()
	var centroid []float32
	for _, ep := range episodes {
		if ep.ID == epID {
			centroid = ep.Centroid
			break
		}
	}
	if len(centroid) == 0 {
		return
	}
	topicID, score := p.topicStoreInst.FindBestTopic(centroid)
	facts := p.episodeStore.GetFacts(epID)
	kid, kscore := p.topicStoreInst.FindBestTopicKeyword(facts)
	if kscore > score {
		topicID, score = kid, kscore
	}
	if score >= 0.3 && topicID > 0 {
		p.topicStoreInst.AssignEpisodeToTopic(epID, topicID)
		p.topicStoreInst.UpdateCentroid(topicID)
	}
	if p.background != nil {
		p.background.NotifyEpisodeCreated()
	}
}
