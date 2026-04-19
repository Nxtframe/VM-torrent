/* globals app */

app.controller("NodeController", function ($scope, $rootScope, $http, $timeout, reqerr) {
  if (!$scope.node) {
    $scope.node = {};
  }
  var n = $scope.node;
  $scope.isfile = function () {
    return !n.Children;
  };
  $scope.isdir = function () {
    return !$scope.isfile();
  };

  $scope.showPreview = false;

  $scope.previewIcon = function () {
    if ($scope.audioPreview) return "play circle blue icon";
    if ($scope.imagePreview) return "eye blue icon";
    if ($scope.videoPreview) return "film blue icon";
    return "play blue icon";
  };

  // Ensure Name is defined, fallback to empty string if missing
  if (typeof n.Name !== 'string') {
    n.Name = "NIGGA";
  }
  // Build path using explicit parentPath passed from templates.
  // This avoids relying on Angular scope chains (which can be ambiguous/inherited).
  var path; // Closure variable accessible to all functions
  var buildPath = function () {
    var parentPath = (typeof n.$parentPath === 'string') ? n.$parentPath : "";
    path = parentPath ? (parentPath + "/" + n.Name) : n.Name;
    var isParent = !!n.Children;
    if (parentPath) {
      console.log('Child node - Name:', n.Name, 'isParent:', isParent, 'parentPath:', parentPath, 'final path:', path);
    } else {
      console.log('Root-level node - Name:', n.Name, 'isParent:', isParent, 'final path:', path);
    }
    n.$path = path;
    $scope.$path = path;
    $scope.audioPreview = /\.(mp3|m4a)$/i.test(path);
    $scope.imagePreview = /\.(jpe?g|png|gif)$/i.test(path);
    $scope.videoPreview = /\.(mp4|mkv|mov|mpeg|ts|avi|webm|ogv|wmv)$/i.test(path);
  };
  buildPath();

  // Watch for parentPath changes (ng-init runs after controller)
  $scope.$watch('node.$parentPath', function (newVal, oldVal) {
    if (newVal !== oldVal) {
      buildPath();
    }
  });
  n.$closed = $scope.agoHrs(n.Modified) > 24;

  $scope.isdownloading = function (fileName) {
    if ($scope.isfile() && (fileName in $rootScope.DownloadingFiles)) {
      return true
    }
    return false
  }

  $scope.preremove = function () {
    $scope.confirm = true;
    $timeout(function () {
      $scope.confirm = false;
    }, 3000);
  };

  //defaults
  $scope.closed = function () {
    return n.$closed;
  };
  $scope.toggle = function () {
    n.$closed = !n.$closed;
  };
  $scope.icon = function () {
    var c = [];
    if ($scope.isdownloading(n.Name)) {
      c.push("spinner", "loading");
    } else {
      c.push("outline");
      if ($scope.isfile()) {
        if ($scope.audioPreview) c.push("audio");
        else if ($scope.imagePreview) c.push("image");
        else if ($scope.videoPreview || /\.(avi|mpeg|ts|webm|ogv)$/.test(path)) c.push("video");
        c.push("file");
      } else {
        c.push("folder");
        if (!$scope.closed()) c.push("open");
      }
    }
    c.push("icon");
    return c.join(" ");
  };

  $scope.remove = function (node) {
    $scope.deleting = true;
    $http.delete("download/" + encodeURIComponent(node.$path))
      .then(function () {
        node.$Deleted = true;
        $scope.$applyAsync();
      })
      .catch(reqerr)
      .finally(function () {
        $scope.deleting = false;
      });
  };

  $scope.videoPlayer = null;
  $scope.theaterVideoPlayer = null;
  $scope.theaterMode = false;
  $scope.theaterActive = false;

  $scope.getVideoPlayerId = function () {
    return 'video-' + n.$path.replace(/[^a-zA-Z0-9]/g, '_');
  };

  $scope.getTheaterVideoPlayerId = function () {
    return 'video-' + n.$path.replace(/[^a-zA-Z0-9]/g, '_') + '-theater';
  };

  $scope.hlsInstance = null;

  $scope.convert = null;
  $scope.convertPoller = null;
  $scope.useConvertedFile = false;
  $scope.canConvertInfo = null;

  // Check if file needs server-side transcoding
  $scope.needsTranscoding = function (filename) {
    return /\.(ts|mpeg|mpg|mkv|avi|mov|wmv)$/i.test(filename);
  };

  // Check if file can be converted (codecs compatible with MP4)
  $scope.checkCanConvert = function () {
    if (!$scope.videoPreview || !$scope.needsTranscoding(n.Name)) {
      $scope.canConvertInfo = { canConvert: false };
      return;
    }
    $http.get('api/canconvert?path=' + encodeURIComponent(n.$path))
      .then(function (xhr) {
        $scope.canConvertInfo = xhr.data;
      })
      .catch(function () {
        $scope.canConvertInfo = { canConvert: true };
      });
  };

  // Determine if convert button should show
  $scope.showConvertButton = function () {
    if (!$scope.videoPreview || !$scope.needsTranscoding(n.Name)) return false;
    if ($scope.canConvertInfo && $scope.canConvertInfo.canConvert === false) return false;
    if (!$scope.convert || $scope.convert.state === 'idle' || $scope.convert.state === 'error') return true;
    return false;
  };

  // Get video URL - direct or transcoded
  $scope.getVideoUrl = function () {
    var encodedPath = encodeURIComponent(n.$path);
    if ($scope.useConvertedFile && $scope.convert && $scope.convert.state === 'done' && $scope.convert.cacheRel) {
      return 'download/' + $scope.convert.cacheRel;
    }
    if ($scope.needsTranscoding(n.Name)) {
      return 'transcode/' + encodedPath;
    }
    return 'download/' + encodedPath;
  };

  $scope.refreshConvertStatus = function () {
    if (!$scope.videoPreview) return;
    $http.get('api/convertstatus?path=' + encodeURIComponent(n.$path))
      .then(function (xhr) {
        $scope.convert = {
          state: xhr.data.state,
          progress: xhr.data.progress || 0,
          error: xhr.data.error,
          cacheRel: xhr.data.cacheRel
        };
        if ($scope.convert.state === 'done') {
          $scope.useConvertedFile = true;
          $scope.reinitVideoPlayer();
          return;
        }
        if ($scope.convert.state === 'running') {
          $scope.pollConvert();
        }
      })
      .catch(function () {
        $scope.convert = { state: 'idle', progress: 0 };
      });
  };

  $scope.startConvert = function () {
    $scope.useConvertedFile = false;
    $scope.convert = { state: 'running', progress: 0 };
    $http.post('api/convert?path=' + encodeURIComponent(n.$path), '')
      .then(function () {
        $scope.pollConvert();
      })
      .catch(function (err) {
        $scope.convert = { state: 'error', progress: 0, error: (err.data || err.statusText) };
      });
  };

  $scope.pollConvert = function () {
    if ($scope.convertPoller) return;
    var tick = function () {
      if (!$scope.showPreview || !$scope.videoPreview) {
        $scope.convertPoller = null;
        return;
      }
      $http.get('api/convertstatus?path=' + encodeURIComponent(n.$path))
        .then(function (xhr) {
          $scope.convert = {
            state: xhr.data.state,
            progress: xhr.data.progress || 0,
            error: xhr.data.error,
            cacheRel: xhr.data.cacheRel
          };
          if ($scope.convert.state === 'done') {
            $scope.useConvertedFile = true;
            $scope.convertPoller = null;
            $scope.reinitVideoPlayer();
            return;
          }
          if ($scope.convert.state === 'error') {
            $scope.convertPoller = null;
            return;
          }
          $scope.convertPoller = $timeout(tick, 1000);
        })
        .catch(function () {
          $scope.convertPoller = $timeout(tick, 1500);
        });
    };
    $scope.convertPoller = $timeout(tick, 500);
  };

  $scope.useConverted = function () {
    $scope.useConvertedFile = true;
    // Open preview if not already open
    if (!$scope.showPreview) {
      $scope.showPreview = true;
      $timeout(function () {
        $scope.videoPlayer = $scope.initVideoPlayer($scope.getVideoPlayerId(), true);
      }, 100);
    } else {
      $scope.reinitVideoPlayer();
    }
  };

  $scope.reinitVideoPlayer = function () {
    if (!$scope.videoPreview) return;
    if ($scope.videoPlayer) {
      $scope.videoPlayer.destroy();
      $scope.videoPlayer = null;
    }
    if ($scope.hlsInstance) {
      $scope.hlsInstance.destroy();
      $scope.hlsInstance = null;
    }
    $timeout(function () {
      $scope.videoPlayer = $scope.initVideoPlayer($scope.getVideoPlayerId(), true);
    }, 50);
  };

  $scope.initVideoPlayer = function (videoId, autoplay) {
    var videoElement = document.getElementById(videoId);
    if (!videoElement || typeof Plyr === 'undefined') return null;

    var isTsFile = /\.ts$/i.test(n.Name);
    var needsTranscode = $scope.needsTranscoding(n.Name);
    var videoUrl = $scope.getVideoUrl();

    // Destroy previous hls instance if exists
    if ($scope.hlsInstance) {
      $scope.hlsInstance.destroy();
      $scope.hlsInstance = null;
    }

    // For .ts files without transcoding, use hls.js
    if (isTsFile && !needsTranscode && typeof Hls !== 'undefined' && Hls.isSupported()) {
      $scope.hlsInstance = new Hls({
        maxBufferLength: 30,
        maxMaxBufferLength: 60
      });
      $scope.hlsInstance.loadSource(videoUrl);
      $scope.hlsInstance.attachMedia(videoElement);
    }

    var player = new Plyr(videoElement, {
      autoplay: autoplay,
      controls: ['play', 'progress', 'current-time', 'duration', 'mute', 'volume', 'settings', 'fullscreen'],
      settings: ['speed', 'quality'],
      speed: { selected: 1, options: [0.5, 0.75, 1, 1.25, 1.5, 2] },
      hideControls: false,
      resetOnEnd: false
    });

    // Keyboard shortcuts for seeking
    var keyHandler = function (e) {
      if (!$scope.videoPlayer) return;

      var seekAmount = 10;
      if (e.key === 'd' || e.key === 'D') {
        $scope.seekRelative(seekAmount);
        e.preventDefault();
      } else if (e.key === 'a' || e.key === 'A') {
        $scope.seekRelative(-seekAmount);
        e.preventDefault();
      } else if (e.key === 'Escape' || e.key === 'Esc') {
        // Close instantly without full toggle
        $scope.showPreview = false;
        if ($scope.videoPlayer) {
          $scope.videoPlayer.destroy();
          $scope.videoPlayer = null;
        }
        if ($scope.hlsInstance) {
          $scope.hlsInstance.destroy();
          $scope.hlsInstance = null;
        }
        $scope.theaterActive = false;
        e.preventDefault();
      }
    };

    // Remove any existing key handler to prevent duplicates
    if ($scope.videoKeyHandler) {
      document.removeEventListener('keydown', $scope.videoKeyHandler);
    }
    $scope.videoKeyHandler = keyHandler;
    document.addEventListener('keydown', keyHandler);

    // Cleanup on destroy
    player.on('destroy', function () {
      document.removeEventListener('keydown', keyHandler);
    });

    return player;
  };

  $scope.toggleTheaterMode = function () {
    var wasPlaying = $scope.videoPlayer && !$scope.videoPlayer.paused;
    var currentTime = $scope.videoPlayer ? $scope.videoPlayer.currentTime : 0;

    $scope.theaterMode = !$scope.theaterMode;

    if ($scope.theaterMode) {
      // Pause inline player, init theater player
      if ($scope.videoPlayer) {
        $scope.videoPlayer.pause();
      }
      $timeout(function () {
        $scope.theaterVideoPlayer = $scope.initVideoPlayer($scope.getTheaterVideoPlayerId(), false);
        if ($scope.theaterVideoPlayer) {
          $scope.theaterVideoPlayer.currentTime = currentTime;
          if (wasPlaying) {
            $scope.theaterVideoPlayer.play();
          }
        }
      }, 100);
    } else {
      // Pause theater player, resume inline player
      if ($scope.theaterVideoPlayer) {
        currentTime = $scope.theaterVideoPlayer.currentTime;
        $scope.theaterVideoPlayer.pause();
      }
      if ($scope.videoPlayer) {
        $scope.videoPlayer.currentTime = currentTime;
        if (wasPlaying) {
          $scope.videoPlayer.play();
        }
      }
    }
  };

  // Simple theater mode toggle - CSS-based approach
  $scope.toggleTheater = function () {
    $scope.theaterActive = !$scope.theaterActive;
  };

  // Subtitles support
  $scope.subtitlesEnabled = false;
  $scope.subtitlesTrack = null;

  $scope.hasSubtitles = function () {
    // Check if a subtitle file exists (.srt, .vtt, .ass, .ssa) with same name
    var basePath = n.$path.replace(/\.[^/.]+$/, "");
    var parent = $scope.$parent.$parent;
    if (!parent || !parent.node || !parent.node.Children) return false;
    
    for (var i = 0; i < parent.node.Children.length; i++) {
      var sibling = parent.node.Children[i];
      if (sibling.Name && sibling.Name.match(/\.(srt|vtt|ass|ssa)$/i)) {
        var siblingBase = sibling.Name.replace(/\.[^/.]+$/, "");
        if (siblingBase === basePath || sibling.Name.indexOf(basePath) === 0) {
          return true;
        }
      }
    }
    return false;
  };

  $scope.toggleSubtitles = function () {
    var player = $scope.theaterActive && $scope.theaterVideoPlayer
      ? $scope.theaterVideoPlayer
      : $scope.videoPlayer;
    if (!player) return;

    if ($scope.subtitlesEnabled) {
      // Disable subtitles
      for (var i = 0; i < player.textTracks.length; i++) {
        player.textTracks[i].mode = 'hidden';
      }
      $scope.subtitlesEnabled = false;
    } else {
      // Enable subtitles
      for (var i = 0; i < player.textTracks.length; i++) {
        player.textTracks[i].mode = 'showing';
      }
      $scope.subtitlesEnabled = true;
    }
  };

  // Seek relative to current position (seconds can be negative)
  $scope.seekRelative = function (seconds) {
    var player = $scope.theaterActive && $scope.theaterVideoPlayer
      ? $scope.theaterVideoPlayer
      : $scope.videoPlayer;
    if (player && player.currentTime !== undefined) {
      player.currentTime = Math.max(0, player.currentTime + seconds);
    }
  };

  // Find next video in the list
  $scope.findNextVideoNode = function () {
    var siblings = [];
    var parent = $scope.$parent.$parent;
    if (parent && parent.node && parent.node.Children) {
      siblings = parent.node.Children;
    } else if ($scope.$parent.$DownloadedFiles) {
      siblings = $scope.$parent.$DownloadedFiles;
    }

    var currentIndex = -1;
    for (var i = 0; i < siblings.length; i++) {
      if (siblings[i].$path === n.$path) {
        currentIndex = i;
        break;
      }
    }

    for (var j = currentIndex + 1; j < siblings.length; j++) {
      if (/\.(mp4|mkv|mov|avi|mpeg|ts|webm|ogv)$/i.test(siblings[j].Name)) {
        return siblings[j];
      }
    }
    return null;
  };

  $scope.hasNextVideo = function () {
    return $scope.findNextVideoNode() !== null;
  };

  $scope.playNextVideo = function () {
    var nextNode = $scope.findNextVideoNode();
    if (!nextNode) return;

    var nextScope = null;
    var siblings = [];
    var parent = $scope.$parent.$parent;
    if (parent && parent.node && parent.node.Children) {
      siblings = parent.node.Children;
    } else if ($scope.$parent.$DownloadedFiles) {
      siblings = $scope.$parent.$DownloadedFiles;
    }

    // Find the scope of the next node
    var childScopes = $scope.$parent.$$childHead;
    while (childScopes) {
      if (childScopes.node && childScopes.node.$path === nextNode.$path) {
        nextScope = childScopes;
        break;
      }
      childScopes = childScopes.$$nextSibling;
    }

    // Close current preview
    $scope.showPreview = false;
    if ($scope.videoPlayer) {
      $scope.videoPlayer.destroy();
      $scope.videoPlayer = null;
    }
    if ($scope.hlsInstance) {
      $scope.hlsInstance.destroy();
      $scope.hlsInstance = null;
    }
    $scope.theaterActive = false;

    // Open next video preview
    if (nextScope) {
      $timeout(function () {
        nextScope.showPreview = true;
        nextScope.$applyAsync();
        $timeout(function () {
          nextScope.videoPlayer = nextScope.initVideoPlayer(nextScope.getVideoPlayerId(), true);
        }, 100);
      }, 50);
    }
  };

  $scope.togglePreview = function () {
    $scope.showPreview = !$scope.showPreview;
    if (!$scope.showPreview) {
      $scope.theaterMode = false;
      if ($scope.theaterVideoPlayer) {
        $scope.theaterVideoPlayer.destroy();
        $scope.theaterVideoPlayer = null;
      }
    }

    // Initialize Plyr for video files
    if ($scope.showPreview && $scope.videoPreview) {
      $scope.useConvertedFile = false;
      $scope.convert = null;
      $scope.canConvertInfo = null;
      $scope.checkCanConvert();
      $scope.refreshConvertStatus();
      $timeout(function () {
        $scope.videoPlayer = $scope.initVideoPlayer($scope.getVideoPlayerId(), true);
      }, 100);
    } else if (!$scope.showPreview && $scope.videoPlayer) {
      $scope.videoPlayer.destroy();
      $scope.videoPlayer = null;
    }
    if (!$scope.showPreview && $scope.hlsInstance) {
      $scope.hlsInstance.destroy();
      $scope.hlsInstance = null;
    }
    if (!$scope.showPreview && $scope.convertPoller) {
      $timeout.cancel($scope.convertPoller);
      $scope.convertPoller = null;
    }
  };

  // Initialize conversion state on controller load (persists across page refresh)
  // Defer to ensure all functions are defined
  if ($scope.videoPreview) {
    $timeout(function () {
      if ($scope.needsTranscoding(n.Name)) {
        $scope.checkCanConvert();
        $scope.refreshConvertStatus();
      }
    }, 0);
  }
});
