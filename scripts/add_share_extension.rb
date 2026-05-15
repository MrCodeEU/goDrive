#!/usr/bin/env ruby
# Adds the ShareExtension target to the Xcode project.
# Runs on the macOS CI runner before flutter build ios.
# Usage: ruby scripts/add_share_extension.rb

require 'xcodeproj'

PROJECT_PATH   = 'mobile/ios/Runner.xcodeproj'
APP_GROUP_ID   = 'group.com.example.godrive'
EXTENSION_NAME = 'ShareExtension'
EXTENSION_ID   = 'com.example.godrive.ShareExtension'
IOS_TARGET     = '16.0'

project = Xcodeproj::Project.open(PROJECT_PATH)

# Skip if already added (idempotent)
if project.targets.any? { |t| t.name == EXTENSION_NAME }
  puts "#{EXTENSION_NAME} target already exists, skipping."
  exit 0
end

# --- Add source files to project ---
ext_group = project.main_group['ShareExtension'] ||
            project.main_group.new_group(EXTENSION_NAME, 'ShareExtension')

swift_ref = ext_group.new_file('ShareExtension/ShareViewController.swift')
plist_ref  = ext_group.new_file('ShareExtension/Info.plist')
ents_ref   = ext_group.new_file('ShareExtension/ShareExtension.entitlements')

# --- Create the extension target ---
ext_target = project.new_target(
  :app_extension, EXTENSION_NAME, :ios, IOS_TARGET,
  project.products_group
)

# Configure build settings for both configurations
ext_target.build_configurations.each do |config|
  config.build_settings['PRODUCT_BUNDLE_IDENTIFIER'] = EXTENSION_ID
  config.build_settings['SWIFT_VERSION']              = '5.0'
  config.build_settings['IPHONEOS_DEPLOYMENT_TARGET'] = IOS_TARGET
  config.build_settings['CODE_SIGN_ENTITLEMENTS'] =
    "ShareExtension/ShareExtension.entitlements"
  config.build_settings['INFOPLIST_FILE'] = 'ShareExtension/Info.plist'
  config.build_settings['SKIP_INSTALL']   = 'YES'
  config.build_settings['SWIFT_EMIT_LOC_STRINGS'] = 'YES'
end

# Add source file to Compile Sources phase
ext_target.source_build_phase.add_file_reference(swift_ref)

# Add Info.plist to Resources phase (suppress warning, it's in build settings)
# Don't add plist to resources — it's handled via INFOPLIST_FILE

# --- Wire entitlements on the extension ---
# (already set in build_settings above)

# --- Add App Group to Runner (main app) entitlements ---
runner_target = project.targets.find { |t| t.name == 'Runner' }
if runner_target
  runner_target.build_configurations.each do |config|
    config.build_settings['CODE_SIGN_ENTITLEMENTS'] = 'Runner/Runner.entitlements'
  end
end

# --- Embed extension in Runner ---
embed_phase = runner_target&.copy_files_build_phases&.find do |p|
  p.name == 'Embed Foundation Extensions' || p.dst_subfolder_spec == '13'
end
unless embed_phase
  embed_phase = runner_target&.new_copy_files_build_phase('Embed Foundation Extensions')
  embed_phase.dst_subfolder_spec = '13' if embed_phase
end

if embed_phase
  ext_ref = ext_target.product_reference
  build_file = embed_phase.add_file_reference(ext_ref)
  build_file.settings = { 'ATTRIBUTES' => ['RemoveHeadersOnCopy'] }
end

project.save
puts "#{EXTENSION_NAME} target added successfully."
