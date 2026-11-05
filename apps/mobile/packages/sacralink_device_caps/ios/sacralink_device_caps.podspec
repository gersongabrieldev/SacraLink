Pod::Spec.new do |s|
  s.name             = 'sacralink_device_caps'
  s.version          = '0.1.0'
  s.summary          = 'Capacidades reais das câmeras e estado térmico (SacraLink Camera).'
  s.description      = 'Lentes físicas, faixa de zoom, fatores ópticos e estado térmico via AVFoundation.'
  s.homepage         = 'https://github.com/gersongabrieldev/SacraLink'
  s.license          = { :type => 'MIT' }
  s.author           = { 'SacraLink' => 'https://github.com/gersongabrieldev/SacraLink' }
  s.source           = { :path => '.' }
  s.source_files     = 'Classes/**/*'
  s.dependency 'Flutter'
  s.platform         = :ios, '14.0'
  s.swift_version    = '5.0'
  s.pod_target_xcconfig = { 'DEFINES_MODULE' => 'YES' }
end
