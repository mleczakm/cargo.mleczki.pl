<?php

namespace App\UI\Http;

use Symfony\Bundle\FrameworkBundle\Controller\AbstractController;
use Symfony\Component\Routing\Attribute\Route;

class HomepageAction  extends AbstractController
{
    #[Route('/', name: 'homepage')]
    public function __invoke()
    {
        return $this->render('base.html.twig');
    }
}