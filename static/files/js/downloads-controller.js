/* globals app */

app.controller("DownloadsController", function ($scope, $rootScope, apiget) {

  $scope.$isLoadingFiles = false;
  $scope.$DownloadedFiles = [];
  $rootScope.$downloadSortBy = 'Modified';
  $rootScope.$downloadSortReverse = true;
  $scope.$sortBy = $rootScope.$downloadSortBy;
  $scope.$sortReverse = $rootScope.$downloadSortReverse;

  apiget.files().then(function (xhr) {
    if (xhr.data.Children) {
      console.log("Files API returned", xhr.data.Children.length, "items");
      var names = xhr.data.Children.map(function(c) { return c.Name; });
      console.log("File names:", names);
      $scope.$DownloadedFiles = xhr.data.Children;
    }
  });

  $scope.$expanded = false;
  $scope.section_expanded_toggle = function () {
    $scope.$expanded = !$scope.$expanded;
    if ($scope.$expanded) {
      $scope.$isLoadingFiles = true;
      apiget.files().then(function (xhr) {
        if (xhr.data.Children) {
          console.log("Files API (expand) returned", xhr.data.Children.length, "items");
          var names = xhr.data.Children.map(function(c) { return c.Name; });
          console.log("File names (expand):", names);
          $scope.$DownloadedFiles = xhr.data.Children;
        } else {
          $scope.$DownloadedFiles = [];
        }
      }).finally(function () {
        $scope.$isLoadingFiles = false;
        $scope.$applyAsync();
      });
    }
  };

  // Expand all folders
  $scope.$expandAll = function() {
    var expandNode = function(node) {
      if (node.Children) {
        node.$closed = false;
        node.Children.forEach(expandNode);
      }
    };
    $scope.$DownloadedFiles.forEach(expandNode);
    $scope.$applyAsync();
  };

  // Collapse all folders
  $scope.$collapseAll = function() {
    var collapseNode = function(node) {
      if (node.Children) {
        node.$closed = true;
        node.Children.forEach(collapseNode);
      }
    };
    $scope.$DownloadedFiles.forEach(collapseNode);
    $scope.$applyAsync();
  };
});
