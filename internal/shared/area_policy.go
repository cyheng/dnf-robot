package shared

const GuildAgitVillage = 8

// GenericAreaAllowed reports whether CMD 38 may be used for a destination.
// Guild agit entry requires its dedicated protocol because df_game_r maps the
// logical area to a guild instance that cannot be represented by generic CMD 38.
func GenericAreaAllowed(guildID, village int) bool {
	return guildID == 0 || village != GuildAgitVillage
}

func FilterGenericAreaMaps(maps []MapCatalogItem, guildID int) []MapCatalogItem {
	if len(maps) == 0 {
		return nil
	}
	out := make([]MapCatalogItem, 0, len(maps))
	for _, mp := range maps {
		if GenericAreaAllowed(guildID, mp.Village) {
			out = append(out, mp)
		}
	}
	return out
}
