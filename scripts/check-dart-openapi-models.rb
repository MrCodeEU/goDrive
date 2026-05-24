#!/usr/bin/env ruby
# frozen_string_literal: true

require "yaml"

SPEC_PATH = "docs/openapi.yaml"
DART_PATH = "mobile/lib/api/models.dart"

MODEL_SCHEMAS = {
  "ListPage" => "ListResponse",
  "User" => "User",
  "FileEntry" => "FileEntry",
  "TrashItem" => "TrashItem",
  "TextPreview" => "TextPreview",
  "ExifData" => "ExifData",
  "AdminJob" => "AdminJob",
  "PreviewToolStatus" => "PreviewToolStatus",
  "APIKey" => "APIKey",
  "Webhook" => "Webhook"
}.freeze

schemas = YAML.load_file(SPEC_PATH).fetch("components").fetch("schemas")
source = File.read(DART_PATH)

missing = []

MODEL_SCHEMAS.each do |class_name, schema_name|
  schema = schemas.fetch(schema_name)
  required_fields = schema.fetch("required", [])

  class_match = source.match(/class #{Regexp.escape(class_name)}\b.*?(?=\nclass |\z)/m)
  unless class_match
    missing << "#{class_name}: class not found"
    next
  end

  class_source = class_match[0]
  required_fields.each do |field|
    next if class_source.include?("j['#{field}']") || class_source.include?('j["#{field}"]')

    missing << "#{class_name}: missing required JSON field #{field.inspect} from #{schema_name}"
  end
end

if missing.empty?
  puts "Dart API models cover required OpenAPI fields"
else
  warn "Dart API model drift detected:"
  missing.each { |item| warn "  #{item}" }
  exit 1
end
