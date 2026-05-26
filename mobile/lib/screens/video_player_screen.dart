import 'package:chewie/chewie.dart';
import 'package:flutter/material.dart';
import 'package:video_player/video_player.dart';

class VideoPlayerScreen extends StatefulWidget {
  final String url;
  final String title;
  final Map<String, String> headers;

  const VideoPlayerScreen({
    super.key,
    required this.url,
    required this.title,
    required this.headers,
  });

  @override
  State<VideoPlayerScreen> createState() => _VideoPlayerScreenState();
}

class _VideoPlayerScreenState extends State<VideoPlayerScreen> {
  late VideoPlayerController _vpc;
  ChewieController? _chewieCtrl;
  String? _error;

  @override
  void initState() {
    super.initState();
    _init();
  }

  Future<void> _init() async {
    try {
      _vpc = VideoPlayerController.networkUrl(
        Uri.parse(widget.url),
        httpHeaders: widget.headers,
      );
      await _vpc.initialize();
      if (!mounted) return;
      _chewieCtrl = ChewieController(
        videoPlayerController: _vpc,
        autoPlay: true,
        looping: false,
        allowFullScreen: true,
        allowMuting: true,
        showControlsOnInitialize: true,
      );
      setState(() {});
    } catch (e) {
      if (mounted) setState(() => _error = e.toString());
    }
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      backgroundColor: Colors.black,
      appBar: AppBar(
        backgroundColor: Colors.black,
        foregroundColor: Colors.white,
        title: Text(widget.title, style: const TextStyle(fontSize: 14)),
      ),
      body: SafeArea(
        child: _error != null
            ? Center(
                child: Text(_error!,
                    style: const TextStyle(color: Colors.white70)))
            : _chewieCtrl == null
                ? const Center(child: CircularProgressIndicator())
                : Chewie(controller: _chewieCtrl!),
      ),
    );
  }

  @override
  void dispose() {
    _chewieCtrl?.dispose();
    _vpc.dispose();
    super.dispose();
  }
}
