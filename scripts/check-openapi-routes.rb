#!/usr/bin/env ruby
# frozen_string_literal: true

require "yaml"

server = File.read("internal/server/server.go")
spec = YAML.load_file("docs/openapi.yaml")

implemented = server.scan(/mux\.HandleFunc\("([A-Z]+) ([^"]+)"/).filter_map do |method, path|
  next if method == "OPTIONS"
  next unless path.start_with?("/api/")

  [method.downcase, path]
end.sort

documented = spec.fetch("paths").flat_map do |path, operations|
  operations.keys
            .select { |method| %w[get post patch delete head put].include?(method) }
            .map { |method| [method, path] }
end.select { |(_, path)| path.start_with?("/api/") }.sort

missing = implemented - documented
extra = documented - implemented

unless missing.empty?
  warn "OpenAPI spec is missing implemented routes:"
  missing.each { |method, path| warn "  #{method.upcase} #{path}" }
end

unless extra.empty?
  warn "OpenAPI spec documents routes not found in server.go:"
  extra.each { |method, path| warn "  #{method.upcase} #{path}" }
end

exit 1 unless missing.empty? && extra.empty?

puts "OpenAPI route coverage OK (#{implemented.length} routes)"
