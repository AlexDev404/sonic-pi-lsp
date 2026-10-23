# frozen_string_literal: true

require "ripper"
require "set"

module Splint
  Diagnostic = Struct.new(:file, :line, :col, :rule, :severity, :message, :fixable, keyword_init: true)

  SEVERITIES = %i[off info warning error].freeze

  def self.severity(v)
    case v.to_s.downcase
    when "off", "0", "false", "none", "disable", "disabled" then :off
    when "info", "hint", "note", "suggestion" then :info
    when "warn", "warning", "1" then :warning
    when "error", "err", "2", "true", "on" then :error
    else raise ConfigError, "unknown severity #{v.inspect} (use off, info, warning or error)"
    end
  end

  def self.rank(sev) = SEVERITIES.index(sev) || 0

  # Where a call runs: the synth in force, the block it is in, and how
  # deeply nested.
  Ctx = Struct.new(:synth, :parent, :loop_name, :loops, :depth) do
    def in_loop? = loops.positive?

    def child(call, k)
      c = dup
      c.parent = call.sym
      c.depth = depth + 1
      case call.sym
      when :live_loop, :loop
        c.loops = loops + 1
        l = call.arg_lit(0)
        c.loop_name = l&.name? ? l.value : "?"
      when :with_synth
        l = call.arg_lit(0)
        c.synth = l&.name? && k.synth?(l.value) ? l.value.to_sym : nil
      when *Rules::DEFINE_FNS
        c.synth = nil
        c.loops = 0
      end
      c
    end
  end

  # One file being linted: its text, its parse, what it defines, and the
  # diagnostics found in it.
  class FileContext
    attr_reader :path, :source, :lines, :sexp, :knowledge, :config, :diagnostics,
                :definitions, :called_names, :symbol_names, :cue_names, :statement_lists,
                :comments, :syntax_errors

    def initialize(path, source, knowledge, config)
      @path = path
      @source = source.encode("UTF-8", invalid: :replace, undef: :replace)
      @lines = @source.split("\n", -1)
      @lines.pop if @lines.last == ""
      @knowledge = knowledge
      @config = config
      @diagnostics = []
      @syntax_errors = Syntax.check(@source, path)
      @sexp = @syntax_errors.empty? ? Ripper.sexp(@source, path) : nil
      @comments = Ripper.lex(@source).select { |_, type, _| type == :on_comment }
                        .map { |(l, c), _, text| [l, c + 1, text.chomp] }
      @rule_options = {}
      collect
    end

    def syntax_error? = !@syntax_errors.empty?

    def report(rule, line, col, message, suggestion: nil)
      sev, = config.rule(rule)
      return if sev == :off

      @diagnostics << Diagnostic.new(file: path, line: line, col: col, rule: rule, severity: sev,
                                     message: message, fixable: Rules.all[rule]&.fixable? || false)
    end

    def defined_names = (@defined_names ||= Set.new(@definitions.keys) | @config.globals | @config.init_definitions)

    def known_function?(name)
      defined_names.include?(name) || knowledge.api?(name) || knowledge.ruby?(name) || @locals.include?(name)
    end

    # Pre-pass: definitions, calls, symbols and cue names in the file.
    def collect
      @definitions = {}
      @called_names = Set.new
      @symbol_names = []
      @cue_names = Set.new
      @locals = Set.new
      @statement_lists = []
      @bodies = {}
      return unless @sexp

      AST.each_node(@sexp) do |n|
        case n[0]
        when :def
          name = AST.ident(n[1])
          l, c = AST.pos(n[1])
          if name
            @definitions[name] ||= [l, c + 1]
            @bodies[name] = n[3]
          end
        when :symbol_literal, :dyna_symbol
          lit = AST.literal(n)
          @symbol_names << lit.value.to_s if lit
        when :params, :block_var
          ps = []
          AST.collect_params(n, ps)
          @locals.merge(ps)
        when :var_field
          id = AST.ident(n[1])
          @locals << id if id
        when :program, :bodystmt
          @statement_lists << n[1] if n[1].is_a?(Array)
        when :do_block, :brace_block
          body = n[2]
          body = body[1] if body.is_a?(Array) && body[0] == :bodystmt
          @statement_lists << body if body.is_a?(Array) && n[2].is_a?(Array) && n[2][0] != :bodystmt
        end
      end
      AST.each_call(@sexp) do |c, _|
        @called_names << c.name if c.fcall? && c.name
        if c.fcall? && Rules::DEFINE_FNS.include?(c.sym)
          l = c.arg_lit(0)
          if l&.name?
            @definitions[l.value.to_s] ||= [c.line, c.col]
            @bodies[l.value.to_s] = c.block&.body
          end
        end
        if c.fcall? && %i[cue set live_loop].include?(c.sym)
          l = c.arg_lit(0)
          @cue_names << l.value.to_s.delete_prefix("/") if l&.name?
        end
      end
    end

    def define_body(name) = @bodies[name.to_s]

    # Whether running these statements advances time: :yes, :no, or :maybe
    # (they call something the linter cannot see into).
    def advances_time(stmts, seen = Set.new)
      result = :no
      walk_calls(stmts) do |c|
        name = c.sym
        if Knowledge::TIME_ADVANCING.include?(name)
          return :yes
        elsif c.fcall? && (body = define_body(c.name))
          next if seen.include?(c.name)

          seen << c.name
          r = advances_time(body, seen)
          return :yes if r == :yes

          result = :maybe if r == :maybe
        elsif c.fcall? && !knowledge.api?(name) && !knowledge.ruby?(name) && !@locals.include?(c.name)
          result = :maybe
        elsif c.fcall? && %i[run_file run_code eval_file method send].include?(name)
          result = :maybe
        elsif !c.fcall? && %i[call].include?(name)
          result = :maybe
        end
      end
      result
    end

    # Calls in statements, not descending into blocks that run in another
    # thread or at another time.
    def walk_calls(stmts, &blk)
      skip = %i[in_thread live_loop at define defonce ndefine lambda proc time_warp].freeze
      visit = lambda do |node|
        next unless node.is_a?(Array)

        if AST::CALL_NODES.include?(node[0]) && (c = AST.call(node))
          blk.call(c)
          c.args.each { |a| visit.call(a) }
          c.opts.each { |o| visit.call(o.value) }
          visit.call(c.recv) if c.recv
          if c.block && !(c.fcall? && skip.include?(c.sym)) && c.sym != :lambda
            c.block.body.each { |s| visit.call(s) }
          end
          next
        end
        next if node[0] == :lambda || node[0] == :def

        node.each { |ch| visit.call(ch) if ch.is_a?(Array) }
      end
      Array(stmts).each { |s| visit.call(s) }
    end

    # Whether a loop body can leave the loop.
    def breaks?(stmts)
      found = false
      AST.each_node(stmts) do |n|
        found = true if %i[break return].include?(n[0])
        found = true if n[0] == :vcall && %w[stop].include?(AST.ident(n[1]))
        found = true if n[0] == :command && %w[raise stop].include?(AST.ident(n[1]))
      end
      found
    end

    def path_exists?(s, search)
      p = File.expand_path(s)
      return true if File.exist?(p)

      dirs = [File.dirname(File.expand_path(path)), Dir.pwd] + Array(search).map { |d| File.expand_path(d) }
      dirs.any? { |d| File.exist?(File.expand_path(s, d)) }
    end
  end

  module Syntax
    module_function

    class Collector < Ripper
      attr_reader :errors

      def initialize(src, file)
        super
        @errors = []
      end

      def on_parse_error(msg)
        @errors << [lineno, column + 1, msg]
      end

      def compile_error(msg)
        @errors << [lineno, column + 1, msg]
      end

      def warn(*); end
      def warning(*); end
    end

    def check(source, path)
      c = Collector.new(source, path)
      c.parse
      c.errors.map { |l, col, msg| [l, col, explain(msg)] }.uniq { |l, _, _| l }.first(5)
    end

    def explain(msg)
      m = msg.to_s.sub(/\Asyntax error, /, "").sub(" or dummy end", "")
      case m
      when /unexpected end-of-input/, /unexpected end of file/, /expected an `end`/
        "#{m} (a block is missing its `end`: every `do` needs one)"
      when /unexpected `end'/, /unexpected 'end'/, /unexpected end\b/
        "#{m} (an extra `end`, or a `do` is missing)"
      else
        m
      end
    end
  end

  # Applies inline directives: # splint:disable, enable, disable-line and
  # disable-next-line.
  module Directives
    RE = /#\s*splint:(disable-next-line|disable-line|disable|enable)\b([^\n]*)/

    module_function

    def parse(file)
      out = []
      file.comments.each do |line, _col, text|
        next unless (m = text.match(RE))

        rules = m[2].split("--").first.to_s.split(/[\s,]+/).reject(&:empty?)
        out << { kind: m[1], rules: rules, line: line, used: false }
      end
      out
    end

    # Removes suppressed diagnostics. Returns [kept, directives].
    def apply(file, diags)
      dirs = parse(file)
      return [diags, dirs] if dirs.empty?

      kept = diags.reject do |d|
        next false if d.rule == "syntax"

        dirs.any? do |dir|
          next false unless dir[:rules].empty? || dir[:rules].include?(d.rule)

          hit = case dir[:kind]
                when "disable-line" then d.line == dir[:line]
                when "disable-next-line" then d.line == dir[:line] + 1
                when "disable" then d.line >= dir[:line] && !enabled_between(dirs, d.rule, dir[:line], d.line)
                else false
                end
          dir[:used] = true if hit
          hit
        end
      end
      [kept, dirs]
    end

    def enabled_between(dirs, rule, from, to)
      dirs.any? { |e| e[:kind] == "enable" && e[:line] > from && e[:line] <= to && (e[:rules].empty? || e[:rules].include?(rule)) }
    end
  end

  # Lints files with a configuration.
  class Linter
    attr_reader :knowledge

    def initialize(knowledge)
      @knowledge = knowledge
    end

    def context(path, source, config) = FileContext.new(path, source, knowledge, config)

    def lint(path, source, config, report_unused_disable: false)
      f = context(path, source, config)
      if f.syntax_error?
        f.syntax_errors.each { |l, c, msg| f.report("syntax", l, c, msg) }
      end
      rules = config.enabled_rules.map { |id| Rules.all[id]&.new(f, config.rule(id)[1]) }.compact
      rules.each(&:on_source)
      if f.sexp
        walk(f.sexp, Ctx.new(:beep, nil, nil, 0, 0), rules)
        rules.each(&:on_finish)
      end
      diags, dirs = config.inline ? Directives.apply(f, f.diagnostics) : [f.diagnostics, []]
      if report_unused_disable
        dirs.each do |d|
          next if d[:used] || d[:kind] == "enable"

          diags << Diagnostic.new(file: path, line: d[:line], col: 1, rule: "unused-disable", severity: :warning,
                                  message: "this splint:#{d[:kind]} comment suppresses nothing", fixable: false)
        end
      end
      diags.uniq { |d| [d.line, d.col, d.rule, d.message] }.sort_by { |d| [d.line, d.col] }
    end

    # Rewrites source with every enabled fixable rule. Returns the new text.
    def fix(path, source, config)
      Rules.all.each_value do |klass|
        next unless klass.fixable? && config.enabled_rules.include?(klass.id)

        f = context(path, source, config)
        next if klass.id == "indentation" && f.syntax_error?

        before = source
        source = klass.new(f, config.rule(klass.id)[1]).fix(source)
        source = before unless source.is_a?(String)
      end
      source
    end

    private

    def walk(node, ctx, rules)
      return unless node.is_a?(Array)

      if AST::CALL_NODES.include?(node[0]) && (c = AST.call(node))
        visit(c, ctx, rules)
        return
      end
      if node[0] == :def
        walk(node[3], Ctx.new(nil, :def, nil, 0, ctx.depth + 1), rules)
        return
      end
      if %i[while until].include?(node[0])
        child = ctx.dup
        child.loops += 1
        child.parent = node[0]
        child.depth += 1
        node[1..].each { |ch| walk(ch, child, rules) }
        return
      end
      node.each { |ch| walk(ch, ctx, rules) if ch.is_a?(Array) }
    end

    def visit(c, ctx, rules)
      rules.each { |r| r.on_call(c, ctx) }
      c.args.each { |a| walk(a, ctx, rules) }
      c.opts.each { |o| walk(o.value, ctx, rules) }
      walk(c.recv, ctx, rules) if c.recv
      if c.block
        child = ctx.child(c, knowledge)
        c.block.body.each { |s| walk(s, child, rules) }
      end
      # use_synth changes the synth for the rest of the enclosing block.
      return unless c.fcall? && c.sym == :use_synth

      l = c.arg_lit(0)
      ctx.synth = l&.name? && knowledge.synth?(l.value) ? l.value.to_sym : nil
    end
  end
end
