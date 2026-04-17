/* globals app */

app.controller("NodeController", function ($scope, $rootScope, $http, $timeout, reqerr) {
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

  var pathArray = [n.Name];
  if ($scope.$parent && $scope.$parent.$parent && $scope.$parent.$parent.node) {
    var parentNode = $scope.$parent.$parent.node;
    pathArray.unshift(parentNode.$path);
    n.$depth = parentNode.$depth + 1;
  } else {
    n.$depth = 1;
  }
  var path = (n.$path = pathArray.join("/"));
  n.$closed = $scope.agoHrs(n.Modified) > 24;
  $scope.audioPreview = /\.(mp3|m4a)$/i.test(path);
  $scope.imagePreview = /\.(jpe?g|png|gif)$/i.test(path);
  $scope.videoPreview = /\.(mp4|mkv|mov)$/i.test(path);

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
        else if ($scope.videoPreview || /\.(avi)$/.test(path)) c.push("video");
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

  $scope.initVideoPlayer = function (videoId, autoplay) {
    var videoElement = document.getElementById(videoId);
    if (videoElement && typeof Plyr !== 'undefined') {
      var player = new Plyr(videoElement, {
        autoplay: autoplay,
        controls: ['play', 'progress', 'current-time', 'duration', 'mute', 'volume', 'settings', 'fullscreen'],
        settings: ['speed', 'quality'],
        speed: { selected: 1, options: [0.5, 0.75, 1, 1.25, 1.5, 2] },
        hideControls: false,
        resetOnEnd: false
      });
      return player;
    }
    return null;
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

  // Seek relative to current position (seconds can be negative)
  $scope.seekRelative = function (seconds) {
    var player = $scope.theaterActive && $scope.theaterVideoPlayer
      ? $scope.theaterVideoPlayer
      : $scope.videoPlayer;
    if (player && player.currentTime !== undefined) {
      player.currentTime = Math.max(0, player.currentTime + seconds);
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
      $timeout(function () {
        $scope.videoPlayer = $scope.initVideoPlayer($scope.getVideoPlayerId(), true);
      }, 100);
    } else if (!$scope.showPreview && $scope.videoPlayer) {
      $scope.videoPlayer.destroy();
      $scope.videoPlayer = null;
    }
  };
});
