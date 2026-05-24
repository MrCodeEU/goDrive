#!/usr/bin/env ruby
# frozen_string_literal: true

require "yaml"

SPEC_PATH = "docs/openapi.yaml"
OUT_PATH = "web/src/lib/api-types.ts"

def ref_name(ref)
  ref.split("/").last
end

def ts_type(schema)
  return "unknown" unless schema
  return ref_name(schema["$ref"]) if schema["$ref"]

  if schema["anyOf"]
    return schema["anyOf"].map { |item| ts_type(item) }.uniq.join(" | ")
  end

  if schema["enum"]
    return schema["enum"].map { |value| value.inspect }.join(" | ")
  end

  case schema["type"]
  when "null"
    "null"
  when "string"
    "string"
  when "integer", "number"
    "number"
  when "boolean"
    "boolean"
  when "array"
    "#{ts_type(schema["items"])}[]"
  when "object", nil
    if schema["additionalProperties"] == true && !schema["properties"]
      "Record<string, unknown>"
    elsif schema["properties"]
      required = schema["required"] || []
      fields = schema["properties"].map do |name, child|
        optional = required.include?(name) ? "" : "?"
        "  #{name}#{optional}: #{ts_type(child)};"
      end
      "{\n#{fields.join("\n")}\n}"
    else
      "Record<string, unknown>"
    end
  else
    "unknown"
  end
end

def generated_content(spec)
  schemas = spec.fetch("components").fetch("schemas")
  lines = [
    "// Generated from docs/openapi.yaml by scripts/generate-openapi-types.rb.",
    "// Do not edit manually. Run `make api-types` after changing the API contract.",
    ""
  ]

  schemas.each do |name, schema|
    lines << "export type #{name} = #{ts_type(schema)};"
    lines << ""
  end

  lines.join("\n")
end

spec = YAML.load_file(SPEC_PATH)
content = generated_content(spec)

if ARGV.include?("--check")
  unless File.exist?(OUT_PATH)
    warn "#{OUT_PATH} does not exist. Run `make api-types`."
    exit 1
  end
  actual = File.read(OUT_PATH)
  if actual != content
    warn "#{OUT_PATH} is out of date. Run `make api-types`."
    exit 1
  end
  puts "OpenAPI TypeScript types are up to date"
  exit 0
end

File.write(OUT_PATH, content)
puts "Generated #{OUT_PATH}"
