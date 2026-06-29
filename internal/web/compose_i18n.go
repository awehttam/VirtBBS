package web

import "encoding/json"

// composeI18nJSON returns localized strings for the web compose editor scripts.
func composeI18nJSON(locale string) string {
	keys := []string{
		"compose.markup_format",
		"compose.markup_format_help",
		"compose.plain_text",
		"compose.stylecodes_label",
		"compose.ansi_label",
		"compose.stylecodes_hint",
		"compose.bold",
		"compose.italic",
		"compose.underline",
		"compose.inverse",
		"compose.advanced_options",
		"compose.hard_wrap",
		"compose.hard_wrap_off",
		"compose.hard_wrap_79",
		"compose.hard_wrap_39",
		"compose.hard_wrap_help",
		"compose.apply_wrap",
		"compose.stats_lines",
		"compose.stats_bytes",
		"compose.body_size_warning",
		"compose.body_too_large",
		"compose.preview",
		"compose.close",
		"ansi_editor.insert_escape_prefix",
		"ansi_editor.insert_sequence",
		"ansi_editor.select_sequence",
		"ansi_editor.custom_sequence",
		"ansi_editor.custom_sequence_placeholder",
		"ansi_editor.cheatsheet_title",
		"ansi_editor.insert_file",
		"ansi_editor.preview_empty",
		"ansi_editor.cheatsheet_help",
		"ansi_editor.cheatsheet_sequence",
		"ansi_editor.cheatsheet_description",
		"ansi_editor.cheatsheet_preview",
		"ansi_editor.group.formatting",
		"ansi_editor.group.foreground",
		"ansi_editor.group.background",
		"ansi_editor.group.cursor",
		"ansi_editor.sequence_reset",
		"ansi_editor.sequence_bold",
		"ansi_editor.sequence_blink",
		"ansi_editor.sequence_reverse",
		"ansi_editor.sequence_fg_black",
		"ansi_editor.sequence_fg_red",
		"ansi_editor.sequence_fg_green",
		"ansi_editor.sequence_fg_yellow",
		"ansi_editor.sequence_fg_blue",
		"ansi_editor.sequence_fg_magenta",
		"ansi_editor.sequence_fg_cyan",
		"ansi_editor.sequence_fg_white",
		"ansi_editor.sequence_bg_black",
		"ansi_editor.sequence_bg_red",
		"ansi_editor.sequence_bg_green",
		"ansi_editor.sequence_bg_yellow",
		"ansi_editor.sequence_bg_blue",
		"ansi_editor.sequence_bg_magenta",
		"ansi_editor.sequence_bg_cyan",
		"ansi_editor.sequence_bg_white",
		"ansi_editor.sequence_clear_screen",
		"ansi_editor.sequence_clear_line",
		"ansi_editor.sequence_cursor_home",
		"ansi_editor.sequence_cursor_save",
		"ansi_editor.sequence_cursor_restore",
		"ansi_editor.sequence_cursor_up",
		"ansi_editor.sequence_cursor_down",
		"ansi_editor.sequence_cursor_right",
		"ansi_editor.sequence_cursor_left",
		"ansi_editor.example_position",
	}
	out := make(map[string]string, len(keys))
	for _, key := range keys {
		short := key
		if i := stringsLastDot(key); i >= 0 {
			short = key[i+1:]
		}
		out[short] = tr(locale, key)
	}
	b, _ := json.Marshal(out)
	return string(b)
}

func stringsLastDot(s string) int {
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == '.' {
			return i
		}
	}
	return -1
}
