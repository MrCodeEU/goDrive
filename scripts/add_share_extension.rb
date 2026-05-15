#!/usr/bin/env ruby
# Adds the ShareExtension target to the Xcode project.
# Does NOT add an embed phase to Runner — that causes a build cycle with
# Flutter's Thin Binary script. Instead, the CI workflow builds the extension
# separately with xcodebuild and embeds it into Runner.app before packaging.
#
# Usage: ruby scripts/add_share_extension.rb

require 'xcodeproj'

PROJECT_PATH   = 'mobile/ios/Runner.xcodeproj'
EXTENSION_NAME = 'ShareExtension'
EXTENSION_ID   = 'com.example.godrive.ShareExtension'
IOS_TARGET     = '16.0'

project = Xcodeproj::Project.open(PROJECT_PATH)

if project.targets.any? { |t| t.name == EXTENSION_NAME }
  puts "#{EXTENSION_NAME} target already exists, skipping."
  exit 0
end

runner_target = project.targets.find { |t| t.name == 'Runner' }
raise 'Runner target not found' unless runner_target

# --- File references ---
ext_group = project.main_group.new_group(EXTENSION_NAME, 'ShareExtension')
swift_ref = ext_group.new_file('ShareViewController.swift')
ext_group.new_file('Info.plist')
ext_group.new_file('ShareExtension.entitlements')

# --- Extension target ---
ext_target = project.new_target(
  :app_extension, EXTENSION_NAME, :ios, IOS_TARGET,
  project.products_group
)

product_ref = ext_target.product_reference
product_ref.name = "#{EXTENSION_NAME}.appex"
product_ref.path = "#{EXTENSION_NAME}.appex"

ext_target.build_configurations.each do |cfg|
  s = cfg.build_settings
  s['PRODUCT_NAME']                = EXTENSION_NAME
  s['PRODUCT_BUNDLE_IDENTIFIER']   = EXTENSION_ID
  s['PRODUCT_BUNDLE_PACKAGE_TYPE'] = 'XPC!'
  s['SWIFT_VERSION']               = '5.0'
  s['IPHONEOS_DEPLOYMENT_TARGET']  = IOS_TARGET
  s['INFOPLIST_FILE']              = 'ShareExtension/Info.plist'
  s['CODE_SIGN_ENTITLEMENTS']      = 'ShareExtension/ShareExtension.entitlements'
  s['SKIP_INSTALL']                = 'YES'
  s['SWIFT_EMIT_LOC_STRINGS']      = 'YES'
  s['LD_RUNPATH_SEARCH_PATHS']     = '$(inherited) @executable_path/Frameworks @executable_path/../../Frameworks'
  s['CODE_SIGN_STYLE']             = 'Automatic'
end

ext_target.source_build_phase.add_file_reference(swift_ref)

# Add Runner.entitlements (App Group) to Runner build settings only.
runner_target.build_configurations.each do |cfg|
  cfg.build_settings['CODE_SIGN_ENTITLEMENTS'] = 'Runner/Runner.entitlements'
end

# Add target dependency so xcodebuild knows to build extension before Runner,
# but do NOT add a Copy Files (embed) phase — that causes a Thin Binary cycle.
runner_target.add_dependency(ext_target)

project.save
puts "#{EXTENSION_NAME} target added successfully (embed handled separately in CI)."
