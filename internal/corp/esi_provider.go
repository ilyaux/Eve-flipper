package corp

import (
	"encoding/json"
	"fmt"
	"sync"

	"eve-flipper/internal/esi"
	"eve-flipper/internal/sde"
)

// ESICorpProvider fetches real corporation data from EVE ESI API.
// Requires a valid access token with Director-level corp scopes.
type ESICorpProvider struct {
	client        *esi.Client
	sdeData       *sde.Data
	accessToken   string
	corporationID int32
	characterID   int64
}

// NewESICorpProvider creates a provider backed by real ESI data.
func NewESICorpProvider(client *esi.Client, sdeData *sde.Data, accessToken string, corporationID int32, characterID int64) *ESICorpProvider {
	return &ESICorpProvider{
		client:        client,
		sdeData:       sdeData,
		accessToken:   accessToken,
		corporationID: corporationID,
		characterID:   characterID,
	}
}

func (e *ESICorpProvider) IsDemo() bool { return false }

func (e *ESICorpProvider) GetInfo() CorpInfo {
	url := fmt.Sprintf("https://esi.evetech.net/latest/corporations/%d/?datasource=tranquility", e.corporationID)
	var info struct {
		Name        string `json:"name"`
		Ticker      string `json:"ticker"`
		MemberCount int    `json:"member_count"`
	}
	if err := e.client.GetJSON(url, &info); err != nil {
		return CorpInfo{CorporationID: e.corporationID}
	}
	return CorpInfo{
		CorporationID: e.corporationID,
		Name:          info.Name,
		Ticker:        info.Ticker,
		MemberCount:   info.MemberCount,
	}
}

func (e *ESICorpProvider) GetWallets() ([]CorpWalletDivision, error) {
	url := fmt.Sprintf("https://esi.evetech.net/latest/corporations/%d/wallets/?datasource=tranquility", e.corporationID)
	var raw []struct {
		Division int     `json:"division"`
		Balance  float64 `json:"balance"`
	}
	if err := e.client.AuthGetJSON(url, e.accessToken, &raw); err != nil {
		return nil, fmt.Errorf("corp wallets: %w", err)
	}

	// Fetch division names
	divNames := e.fetchDivisionNames()

	wallets := make([]CorpWalletDivision, len(raw))
	for i, w := range raw {
		name := fmt.Sprintf("Division %d", w.Division)
		if n, ok := divNames[w.Division]; ok && n != "" {
			name = n
		}
		wallets[i] = CorpWalletDivision{
			Division: w.Division,
			Name:     name,
			Balance:  w.Balance,
		}
	}
	return wallets, nil
}

func (e *ESICorpProvider) fetchDivisionNames() map[int]string {
	url := fmt.Sprintf("https://esi.evetech.net/latest/corporations/%d/divisions/?datasource=tranquility", e.corporationID)
	var raw struct {
		Wallet []struct {
			Division int    `json:"division"`
			Name     string `json:"name"`
		} `json:"wallet"`
	}
	names := make(map[int]string)
	if err := e.client.AuthGetJSON(url, e.accessToken, &raw); err != nil {
		return names
	}
	for _, d := range raw.Wallet {
		names[d.Division] = d.Name
	}
	return names
}

func (e *ESICorpProvider) GetJournal(division int, days int) ([]CorpJournalEntry, error) {
	url := fmt.Sprintf("https://esi.evetech.net/latest/corporations/%d/wallets/%d/journal/?datasource=tranquility", e.corporationID, division)
	rawPages, err := e.client.AuthGetPaginated(url, e.accessToken)
	if err != nil {
		return nil, fmt.Errorf("corp journal div %d: %w", division, err)
	}

	var entries []CorpJournalEntry
	var partyIDs []int64
	for _, page := range rawPages {
		var entry struct {
			ID            int64   `json:"id"`
			Date          string  `json:"date"`
			RefType       string  `json:"ref_type"`
			Amount        float64 `json:"amount"`
			Balance       float64 `json:"balance"`
			Description   string  `json:"description"`
			FirstPartyID  int64   `json:"first_party_id"`
			SecondPartyID int64   `json:"second_party_id"`
		}
		if err := json.Unmarshal(page, &entry); err != nil {
			continue
		}
		if entry.FirstPartyID > 0 {
			partyIDs = append(partyIDs, entry.FirstPartyID)
		}
		if entry.SecondPartyID > 0 {
			partyIDs = append(partyIDs, entry.SecondPartyID)
		}
		entries = append(entries, CorpJournalEntry{
			ID:            entry.ID,
			Date:          entry.Date,
			RefType:       entry.RefType,
			Amount:        entry.Amount,
			Balance:       entry.Balance,
			Description:   entry.Description,
			FirstPartyID:  entry.FirstPartyID,
			SecondPartyID: entry.SecondPartyID,
		})
	}

	// Resolve party names
	names := e.resolveCharacterNames(partyIDs)
	for i := range entries {
		if n, ok := names[entries[i].FirstPartyID]; ok {
			entries[i].FirstPartyName = n
		}
		if n, ok := names[entries[i].SecondPartyID]; ok {
			entries[i].SecondPartyName = n
		}
	}

	return entries, nil
}

func (e *ESICorpProvider) GetTransactions(division int) ([]CorpTransaction, error) {
	url := fmt.Sprintf("https://esi.evetech.net/latest/corporations/%d/wallets/%d/transactions/?datasource=tranquility", e.corporationID, division)
	var raw []struct {
		TransactionID int64   `json:"transaction_id"`
		Date          string  `json:"date"`
		TypeID        int32   `json:"type_id"`
		Quantity      int32   `json:"quantity"`
		UnitPrice     float64 `json:"unit_price"`
		IsBuy         bool    `json:"is_buy"`
		LocationID    int64   `json:"location_id"`
		ClientID      int64   `json:"client_id"`
	}
	if err := e.client.AuthGetJSON(url, e.accessToken, &raw); err != nil {
		return nil, fmt.Errorf("corp transactions div %d: %w", division, err)
	}

	// Collect client IDs for name resolution
	var clientIDs []int64
	for _, t := range raw {
		if t.ClientID > 0 {
			clientIDs = append(clientIDs, t.ClientID)
		}
	}
	clientNames := e.resolveCharacterNames(clientIDs)

	txns := make([]CorpTransaction, len(raw))
	for i, t := range raw {
		txns[i] = CorpTransaction{
			TransactionID: t.TransactionID,
			Date:          t.Date,
			TypeID:        t.TypeID,
			TypeName:      e.typeName(t.TypeID),
			Quantity:      t.Quantity,
			UnitPrice:     t.UnitPrice,
			IsBuy:         t.IsBuy,
			LocationID:    t.LocationID,
			LocationName:  e.client.StationName(t.LocationID),
			ClientID:      t.ClientID,
			ClientName:    clientNames[t.ClientID],
		}
	}
	return txns, nil
}

func (e *ESICorpProvider) GetMembers() ([]CorpMember, error) {
	// Fetch member IDs
	memberURL := fmt.Sprintf("https://esi.evetech.net/latest/corporations/%d/members/?datasource=tranquility", e.corporationID)
	var memberIDs []int64
	if err := e.client.AuthGetJSON(memberURL, e.accessToken, &memberIDs); err != nil {
		return nil, fmt.Errorf("corp members: %w", err)
	}

	// Fetch member tracking data
	trackURL := fmt.Sprintf("https://esi.evetech.net/latest/corporations/%d/membertracking/?datasource=tranquility", e.corporationID)
	var tracking []struct {
		CharacterID int64  `json:"character_id"`
		LastLogin   string `json:"start_date"` // ESI calls it start_date
		LogoffDate  string `json:"logoff_date"`
		ShipTypeID  int32  `json:"ship_type_id"`
		LocationID  int64  `json:"location_id"`
		SystemID    int32  `json:"system_id"`
	}
	_ = e.client.AuthGetJSON(trackURL, e.accessToken, &tracking)

	trackMap := make(map[int64]int)
	for i, t := range tracking {
		trackMap[t.CharacterID] = i
	}

	// Resolve character names in parallel (batch)
	names := e.resolveCharacterNames(memberIDs)

	members := make([]CorpMember, len(memberIDs))
	for i, id := range memberIDs {
		m := CorpMember{
			CharacterID: id,
			Name:        names[id],
		}
		if idx, ok := trackMap[id]; ok {
			t := tracking[idx]
			m.LastLogin = t.LastLogin
			m.LogoffDate = t.LogoffDate
			m.ShipTypeID = t.ShipTypeID
			m.ShipName = e.typeName(t.ShipTypeID)
			m.LocationID = t.LocationID
			m.SystemID = t.SystemID
			if sys, ok := e.sdeData.Systems[t.SystemID]; ok {
				m.SystemName = sys.Name
			}
		}
		members[i] = m
	}
	return members, nil
}

func (e *ESICorpProvider) GetIndustryJobs() ([]CorpIndustryJob, error) {
	url := fmt.Sprintf("https://esi.evetech.net/latest/corporations/%d/industry/jobs/?datasource=tranquility&include_completed=true", e.corporationID)
	var raw []struct {
		JobID           int32  `json:"job_id"`
		InstallerID     int64  `json:"installer_id"`
		ActivityID      int    `json:"activity_id"`
		BlueprintTypeID int32  `json:"blueprint_type_id"`
		ProductTypeID   int32  `json:"product_type_id"`
		Status          string `json:"status"`
		Runs            int32  `json:"runs"`
		StartDate       string `json:"start_date"`
		EndDate         string `json:"end_date"`
		FacilityID      int64  `json:"facility_id"`
	}
	if err := e.client.AuthGetJSON(url, e.accessToken, &raw); err != nil {
		return nil, fmt.Errorf("corp industry: %w", err)
	}

	// Collect installer IDs for name resolution
	var installerIDs []int64
	for _, j := range raw {
		if j.InstallerID > 0 {
			installerIDs = append(installerIDs, j.InstallerID)
		}
	}
	installerNames := e.resolveCharacterNames(installerIDs)

	activityNames := map[int]string{
		1: "manufacturing",
		3: "researching_time_efficiency",
		4: "researching_material_efficiency",
		5: "copying",
		8: "invention",
		9: "reaction",
	}

	jobs := make([]CorpIndustryJob, len(raw))
	for i, j := range raw {
		activity := activityNames[j.ActivityID]
		if activity == "" {
			activity = fmt.Sprintf("activity_%d", j.ActivityID)
		}
		jobs[i] = CorpIndustryJob{
			JobID:           j.JobID,
			InstallerID:     j.InstallerID,
			InstallerName:   installerNames[j.InstallerID],
			Activity:        activity,
			BlueprintTypeID: j.BlueprintTypeID,
			ProductTypeID:   j.ProductTypeID,
			ProductName:     e.typeName(j.ProductTypeID),
			Status:          j.Status,
			Runs:            j.Runs,
			StartDate:       j.StartDate,
			EndDate:         j.EndDate,
			LocationID:      j.FacilityID,
			LocationName:    e.client.StationName(j.FacilityID),
		}
	}
	return jobs, nil
}

func (e *ESICorpProvider) GetMiningLedger() ([]CorpMiningEntry, error) {
	// First, get mining observers
	obsURL := fmt.Sprintf("https://esi.evetech.net/latest/corporation/%d/mining/observers/?datasource=tranquility", e.corporationID)
	var observers []struct {
		ObserverID int64 `json:"observer_id"`
	}
	if err := e.client.AuthGetJSON(obsURL, e.accessToken, &observers); err != nil {
		return nil, fmt.Errorf("mining observers: %w", err)
	}

	var allEntries []CorpMiningEntry
	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, obs := range observers {
		wg.Add(1)
		go func(obsID int64) {
			defer wg.Done()
			url := fmt.Sprintf("https://esi.evetech.net/latest/corporation/%d/mining/observers/%d/?datasource=tranquility", e.corporationID, obsID)
			var raw []struct {
				CharacterID int64  `json:"character_id"`
				RecordedID  int32  `json:"recorded_corporation_id"`
				TypeID      int32  `json:"type_id"`
				Quantity    int64  `json:"quantity"`
				LastUpdated string `json:"last_updated"`
			}
			if err := e.client.AuthGetJSON(url, e.accessToken, &raw); err != nil {
				return
			}
			var entries []CorpMiningEntry
			for _, r := range raw {
				entries = append(entries, CorpMiningEntry{
					CharacterID: r.CharacterID,
					Date:        r.LastUpdated,
					TypeID:      r.TypeID,
					TypeName:    e.typeName(r.TypeID),
					Quantity:    r.Quantity,
				})
			}
			mu.Lock()
			allEntries = append(allEntries, entries...)
			mu.Unlock()
		}(obs.ObserverID)
	}
	wg.Wait()

	// Resolve character names for all mining entries
	var minerIDs []int64
	for _, entry := range allEntries {
		if entry.CharacterID > 0 {
			minerIDs = append(minerIDs, entry.CharacterID)
		}
	}
	minerNames := e.resolveCharacterNames(minerIDs)
	for i := range allEntries {
		if n, ok := minerNames[allEntries[i].CharacterID]; ok {
			allEntries[i].CharacterName = n
		}
	}

	return allEntries, nil
}

func (e *ESICorpProvider) GetOrders() ([]CorpMarketOrder, error) {
	url := fmt.Sprintf("https://esi.evetech.net/latest/corporations/%d/orders/?datasource=tranquility", e.corporationID)
	rawPages, err := e.client.AuthGetPaginated(url, e.accessToken)
	if err != nil {
		return nil, fmt.Errorf("corp orders: %w", err)
	}

	var orders []CorpMarketOrder
	var charIDs []int64
	for _, page := range rawPages {
		var o struct {
			OrderID      int64   `json:"order_id"`
			CharacterID  int64   `json:"issued_by"`
			TypeID       int32   `json:"type_id"`
			Price        float64 `json:"price"`
			VolumeRemain int32   `json:"volume_remain"`
			VolumeTotal  int32   `json:"volume_total"`
			IsBuyOrder   bool    `json:"is_buy_order"`
			LocationID   int64   `json:"location_id"`
			Issued       string  `json:"issued"`
			Duration     int     `json:"duration"`
			RegionID     int32   `json:"region_id"`
		}
		if err := json.Unmarshal(page, &o); err != nil {
			continue
		}
		if o.CharacterID > 0 {
			charIDs = append(charIDs, o.CharacterID)
		}
		orders = append(orders, CorpMarketOrder{
			OrderID:      o.OrderID,
			CharacterID:  o.CharacterID,
			TypeID:       o.TypeID,
			TypeName:     e.typeName(o.TypeID),
			Price:        o.Price,
			VolumeRemain: o.VolumeRemain,
			VolumeTotal:  o.VolumeTotal,
			IsBuyOrder:   o.IsBuyOrder,
			LocationID:   o.LocationID,
			LocationName: e.client.StationName(o.LocationID),
			Issued:       o.Issued,
			Duration:     o.Duration,
			RegionID:     o.RegionID,
		})
	}

	// Resolve character names for order issuers
	charNames := e.resolveCharacterNames(charIDs)
	for i := range orders {
		if n, ok := charNames[orders[i].CharacterID]; ok {
			orders[i].CharacterName = n
		}
	}

	return orders, nil
}

// ============================================================
// Helpers
// ============================================================

func (e *ESICorpProvider) typeName(typeID int32) string {
	if e.sdeData != nil {
		if t, ok := e.sdeData.Types[typeID]; ok {
			return t.Name
		}
	}
	return fmt.Sprintf("Type #%d", typeID)
}

func (e *ESICorpProvider) resolveCharacterNames(ids []int64) map[int64]string {
	names := make(map[int64]string)
	if len(ids) == 0 {
		return names
	}

	// Deduplicate IDs
	seen := make(map[int64]bool)
	var unique []int64
	for _, id := range ids {
		if id > 0 && !seen[id] {
			seen[id] = true
			unique = append(unique, id)
		}
	}

	// ESI POST /universe/names/ accepts up to 1000 IDs per call
	batchSize := 1000
	for start := 0; start < len(unique); start += batchSize {
		end := start + batchSize
		if end > len(unique) {
			end = len(unique)
		}
		batch := unique[start:end]

		// Convert to int32 IDs for the endpoint (safe for character/corp/alliance IDs)
		intIDs := make([]int32, len(batch))
		for i, id := range batch {
			intIDs[i] = int32(id)
		}

		url := "https://esi.evetech.net/latest/universe/names/?datasource=tranquility"
		var results []struct {
			ID   int64  `json:"id"`
			Name string `json:"name"`
		}
		if err := e.client.PostJSON(url, intIDs, &results); err == nil {
			for _, r := range results {
				names[r.ID] = r.Name
			}
		}
	}

	// Fallback for any IDs the API didn't resolve
	for _, id := range ids {
		if _, ok := names[id]; !ok {
			names[id] = fmt.Sprintf("Character %d", id)
		}
	}
	return names
}
