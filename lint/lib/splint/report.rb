# frozen_string_literal: true

require "json"
require "cgi"

module Splint
  # Renders diagnostics.
  module Report
    module_function

    COLORS = { error: "\e[31m", warning: "\e[33m", info: "\e[36m" }.freeze

    def render(format, diags, sources, color: false, stats: false)
      case format
      when "pretty" then pretty(diags, sources, color, stats)
      when "compact" then compact(diags)
      when "json" then json(diags)
      when "github" then github(diags)
      when "sarif" then sarif(diags)
      when "checkstyle" then checkstyle(diags)
      else raise ConfigError, "unknown format #{format}"
      end
    end

    def counts(diags)
      %i[error warning info].to_h { |s| [s, diags.count { |d| d.severity == s }] }
    end

    def paint(color, code, s) = color ? "#{code}#{s}\e[0m" : s

    def pretty(diags, sources, color, stats)
      out = +""
      diags.group_by(&:file).each do |file, ds|
        out << paint(color, "\e[1m\e[4m", file) << "\n"
        lines = (sources[file] || "").split("\n", -1)
        ds.each do |d|
          sev = paint(color, COLORS[d.severity], d.severity.to_s.ljust(7))
          out << format("  %<pos>-8s %<sev>s %<msg>s  %<rule>s\n",
                        pos: "#{d.line}:#{d.col}", sev: sev, msg: d.message, rule: paint(color, "\e[2m", d.rule))
          src = lines[d.line - 1]
          next unless src && !src.strip.empty? && d.rule != "line-length"

          shown = src.length > 160 ? "#{src[0, 157]}..." : src
          out << paint(color, "\e[2m", format("  %5d | ", d.line)) << shown << "\n"
          out << paint(color, "\e[2m", "        | ") << " " * [d.col - 1, 0].max << paint(color, COLORS[d.severity], "^") << "\n"
        end
        out << "\n"
      end
      c = counts(diags)
      total = diags.size
      if total.zero?
        out << paint(color, "\e[32m", "✓ no problems") << "\n"
      else
        word = total == 1 ? "problem" : "problems"
        summary = "✖ #{total} #{word} (#{c[:error]} #{c[:error] == 1 ? 'error' : 'errors'}, #{c[:warning]} #{c[:warning] == 1 ? 'warning' : 'warnings'}, #{c[:info]} info)"
        out << paint(color, c[:error].positive? ? COLORS[:error] : COLORS[:warning], summary) << "\n"
        fixable = diags.count(&:fixable)
        out << "  #{fixable} fixable with `sonicpi lint --fix`\n" if fixable.positive?
      end
      if stats && total.positive?
        out << "\nBy rule:\n"
        diags.group_by(&:rule).sort_by { |_, v| -v.size }.each { |r, v| out << format("  %4d  %s\n", v.size, r) }
      end
      out
    end

    def compact(diags)
      diags.map { |d| "#{d.file}:#{d.line}:#{d.col}: #{d.severity}: #{d.message} [#{d.rule}]\n" }.join
    end

    def json(diags)
      c = counts(diags)
      JSON.pretty_generate(
        "problems" => diags.map do |d|
          { "file" => d.file, "line" => d.line, "column" => d.col, "severity" => d.severity.to_s,
            "rule" => d.rule, "message" => d.message, "fixable" => d.fixable }
        end,
        "errors" => c[:error], "warnings" => c[:warning], "info" => c[:info]
      ) + "\n"
    end

    def github(diags)
      diags.map do |d|
        level = { error: "error", warning: "warning", info: "notice" }[d.severity]
        msg = d.message.gsub("%", "%25").gsub("\n", "%0A")
        "::#{level} file=#{d.file},line=#{d.line},col=#{d.col},title=#{d.rule}::#{msg}\n"
      end.join
    end

    def sarif(diags)
      rules = diags.map(&:rule).uniq.map do |id|
        k = Rules.all[id]
        { "id" => id, "shortDescription" => { "text" => k ? k.summary : id },
          "helpUri" => "https://github.com/AlexDev404/sonic-pi-lsp/blob/claude/sonic-pi-cli/docs/lint-rules.md##{id}" }
      end
      JSON.pretty_generate(
        "version" => "2.1.0",
        "$schema" => "https://json.schemastore.org/sarif-2.1.0.json",
        "runs" => [{
          "tool" => { "driver" => { "name" => "splint", "informationUri" => "https://github.com/AlexDev404/sonic-pi-lsp", "rules" => rules } },
          "results" => diags.map do |d|
            { "ruleId" => d.rule, "level" => { error: "error", warning: "warning", info: "note" }[d.severity],
              "message" => { "text" => d.message },
              "locations" => [{ "physicalLocation" => { "artifactLocation" => { "uri" => d.file },
                                                         "region" => { "startLine" => d.line, "startColumn" => d.col } } }] }
          end
        }]
      ) + "\n"
    end

    def checkstyle(diags)
      out = +%(<?xml version="1.0" encoding="UTF-8"?>\n<checkstyle version="4.3">\n)
      diags.group_by(&:file).each do |file, ds|
        out << %(  <file name="#{CGI.escapeHTML(file)}">\n)
        ds.each do |d|
          out << %(    <error line="#{d.line}" column="#{d.col}" severity="#{d.severity == :info ? 'info' : d.severity}" message="#{CGI.escapeHTML(d.message)}" source="splint.#{d.rule}"/>\n)
        end
        out << "  </file>\n"
      end
      out << "</checkstyle>\n"
    end
  end

  # Sonic Pi's auto-align: the same ruby-beautify pass the server runs when
  # the desktop's Run (or Alt-M) aligns a buffer.
  module Format
    TMP = "_____sonic_pi_tmp_insert_____"

    module_function

    def beautify(_knowledge, source)
      require "ruby-beautify"
      return source if source.strip.empty?

      final_newline = source.end_with?("\n")
      lines = source.lines.map { |l| l.match?(/\A\s*\z/) ? "#{TMP}\n" : l }
      buf = lines.join
      buf << "\n" unless buf.end_with?("\n")
      buf = buf.gsub(%r{\)/}, ") ___SONIC_PI_RND_TMP_PLACEHOLDER___ /").gsub(%r{\]/}, "] ___SONIC_PI_SQR_TMP_PLACEHOLDER___ /")
      res = RBeautify.beautify_string(:ruby, buf)
      res = res.gsub(") ___SONIC_PI_RND_TMP_PLACEHOLDER___ /", ")/").gsub("] ___SONIC_PI_SQR_TMP_PLACEHOLDER___ /", "]/")
      out = res.lines.map { |l| l.include?(TMP) ? "\n" : l }.join
      out = out.chomp
      final_newline ? "#{out}\n" : out
    end
  end
end
