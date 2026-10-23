# frozen_string_literal: true

require "ripper"

module Splint
  # A light view of Ripper's S-expressions: method calls with their
  # arguments, keyword opts and blocks, and literal values.
  module AST
    Lit = Struct.new(:kind, :value) do
      def num? = kind == :num
      def sym? = kind == :sym
      def str? = kind == :str
      def name? = sym? || str?
    end

    Opt = Struct.new(:key, :value, :line, :col)

    Block = Struct.new(:params, :body, :line, :col, :node)

    Call = Struct.new(:name, :recv, :args, :opts, :block, :line, :col, :node, :splat) do
      def fcall? = recv.nil?
      def sym = name&.to_sym
      def arg(i) = args[i]
      def arg_lit(i) = args[i] && AST.literal(args[i])
      def opt(key) = opts.find { |o| o.key == key.to_s }
      def opts_hash
        opts.each_with_object({}) do |o, h|
          l = AST.literal(o.value)
          h[o.key.to_sym] = l.value if l && l.kind != :array
        end
      end
    end

    module_function

    CALL_NODES = %i[command fcall vcall method_add_arg method_add_block command_call call].freeze

    def pos(node)
      return nil unless node.is_a?(Array)
      return node[2] if node[0].is_a?(Symbol) && node[0].to_s.start_with?("@") && node[2].is_a?(Array)

      node.each do |c|
        p = pos(c)
        return p if p
      end
      nil
    end

    def ident(node)
      return nil unless node.is_a?(Array)

      case node[0]
      when :@ident, :@const, :@kw, :@op, :@label
        node[1].to_s.delete_suffix(":")
      end
    end

    # Returns the Call a node represents, or nil.
    def call(node)
      return nil unless node.is_a?(Array) && CALL_NODES.include?(node[0])

      case node[0]
      when :command
        c = new_call(node[1], nil, node)
        fill_args(c, node[2])
        c
      when :fcall, :vcall
        new_call(node[1], nil, node)
      when :method_add_arg
        c = call(node[1])
        return nil unless c

        c.node = node
        fill_args(c, node[2])
        c
      when :method_add_block
        c = call(node[1])
        return nil unless c

        c.node = node
        c.block = block(node[2])
        c
      when :command_call
        c = new_call(node[3], node[1], node)
        fill_args(c, node[4])
        c.block = block(node[5]) if node[5]
        c
      when :call
        name = node[3] == :call ? nil : node[3]
        c = new_call(name, node[1], node)
        c.name ||= "call"
        c
      end
    end

    def new_call(id, recv, node)
      l, col = pos(id) || pos(node) || [0, 0]
      Call.new(ident(id), recv, [], [], nil, l, col + 1, node, false)
    end

    def fill_args(c, node)
      return unless node.is_a?(Array)

      case node[0]
      when :arg_paren
        fill_args(c, node[1])
      when :args_add_block
        list(c, node[1])
        c.splat = true if node[2] && node[2] != false
      when :args_add_star
        c.splat = true
        list(c, node[1])
        node[3..]&.each { |a| add_arg(c, a) }
      else
        list(c, node) if node.all? { |x| x.is_a?(Array) }
      end
    end

    def list(c, items)
      return unless items.is_a?(Array)

      if items[0] == :args_add_star
        c.splat = true
        list(c, items[1])
        items[3..]&.each { |a| add_arg(c, a) }
        return
      end
      items.each { |a| add_arg(c, a) }
    end

    def add_arg(c, a)
      return unless a.is_a?(Array)

      case a[0]
      when :bare_assoc_hash
        assocs(c, a[1])
      when :hash
        # play 60, {amp: 1}
        inner = a[1]
        inner = inner[1] if inner.is_a?(Array) && inner[0] == :assoclist_from_args
        assocs(c, inner)
      else
        c.args << a
      end
    end

    def assocs(c, list)
      return unless list.is_a?(Array)

      list.each do |as|
        next unless as.is_a?(Array)

        if as[0] == :assoc_splat || as[0] == :assoc_splat_kw
          c.splat = true
          next
        end
        next unless as[0] == :assoc_new

        key = as[1]
        name = case key[0]
               when :@label then key[1].delete_suffix(":")
               else
                 l = literal(key)
                 l&.name? ? l.value.to_s : nil
               end
        next unless name

        l, col = pos(key) || [c.line, c.col - 1]
        # Ruby 3.1 shorthand (amp:) has no value node.
        c.opts << Opt.new(name, as[2] || [:var_ref, [:@ident, name, [l, col]]], l, col + 1)
      end
    end

    def block(node)
      return nil unless node.is_a?(Array)

      case node[0]
      when :do_block, :brace_block
        params = []
        collect_params(node[1], params)
        body = node[2]
        body = body[1] if body.is_a?(Array) && body[0] == :bodystmt
        l, col = pos(node) || [0, 0]
        Block.new(params, body.is_a?(Array) ? body : [], l, col + 1, node)
      end
    end

    def collect_params(node, out)
      return unless node.is_a?(Array)

      if node[0] == :@ident
        out << node[1]
        return
      end
      node.each { |c| collect_params(c, out) }
    end

    # The literal value of a node, or nil when it is not a literal.
    def literal(node)
      return nil unless node.is_a?(Array)

      case node[0]
      when :@int
        Lit.new(:num, Integer(node[1].delete("_"), exception: false) || Float(node[1]))
      when :@float
        Lit.new(:num, Float(node[1].delete("_")))
      when :unary
        inner = literal(node[2])
        return Lit.new(:num, -inner.value) if node[1] == :-@ && inner&.num?

        nil
      when :symbol_literal
        sym = node[1]
        sym = sym[1] if sym.is_a?(Array) && sym[0] == :symbol
        name = ident(sym)
        name ? Lit.new(:sym, name.to_sym) : nil
      when :dyna_symbol
        s = string_content(node[1])
        s ? Lit.new(:sym, s.to_sym) : nil
      when :string_literal
        s = string_content(node[1])
        s ? Lit.new(:str, s) : nil
      when :var_ref
        case node[1]
        in [:@kw, "nil", _] then Lit.new(:nil, nil)
        in [:@kw, "true", _] then Lit.new(:bool, true)
        in [:@kw, "false", _] then Lit.new(:bool, false)
        else nil
        end
      when :array
        items = node[1]
        return Lit.new(:array, []) if items.nil?
        return nil unless items.is_a?(Array) && items.all? { |x| x.is_a?(Array) }

        vals = items.map { |x| literal(x) }
        vals.all? ? Lit.new(:array, vals) : nil
      when :paren
        inner = node[1]
        inner = inner[0] if inner.is_a?(Array) && inner.size == 1 && inner[0].is_a?(Array)
        literal(inner)
      end
    end

    def string_content(node)
      return nil unless node.is_a?(Array)

      parts = node[0] == :string_content ? node[1..] : node
      return "" if parts.empty?
      return nil unless parts.all? { |p| p.is_a?(Array) && p[0] == :@tstring_content }

      parts.map { |p| p[1] }.join
    end

    # Visits every node depth first.
    def each_node(node, &blk)
      return unless node.is_a?(Array)

      yield node if node[0].is_a?(Symbol)
      node.each { |c| each_node(c, &blk) if c.is_a?(Array) }
    end

    # Every call in a subtree, each once (a call's callee node is not
    # visited again on its own).
    def each_call(node, &blk)
      return unless node.is_a?(Array)

      if CALL_NODES.include?(node[0]) && (c = call(node))
        yield c, node
        c.args.each { |a| each_call(a, &blk) }
        c.opts.each { |o| each_call(o.value, &blk) }
        each_call(c.recv, &blk) if c.recv
        c.block&.body&.each { |st| each_call(st, &blk) }
        return
      end
      node.each { |ch| each_call(ch, &blk) if ch.is_a?(Array) }
    end
  end
end
